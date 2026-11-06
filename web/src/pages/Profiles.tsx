import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { useLive } from "../state/LiveProvider";
import type { Profile } from "../api/types";
import { ConfirmDialog } from "../components/Modal";
import { Empty, PageHeader } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { RunProfileDialog } from "../components/RunProfileDialog";
import { profileSeconds, workloadLabel } from "../domain";
import { fmtDateTime, fmtDuration, fmtRelative } from "../format";

export function Profiles() {
  const nav = useNavigate();
  const { agents } = useLive();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [q, setQ] = useState("");
  const [fire, setFire] = useState<Profile | null>(null);
  const [del, setDel] = useState<Profile | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () =>
    api
      .profiles()
      .then((p) => setProfiles(p ?? []))
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));

  useEffect(() => {
    load();
  }, []);

  const shown = useMemo(
    () =>
      profiles.filter((p) => (p.name + " " + p.description).toLowerCase().includes(q.trim().toLowerCase())),
    [profiles, q],
  );
  const onlineHosts = agents.filter((a) => a.status !== "offline" && a.capabilities.fio).length;

  async function remove() {
    if (!del) return;
    setBusy(true);
    try {
      await api.deleteProfile(del.id);
      setDel(null);
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function duplicate(p: Profile) {
    try {
      const created = await api.createProfile({
        name: `${p.name} (Kopie)`,
        description: p.description,
        steps: p.steps,
      });
      nav(`/profiles/${created.id}`);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <div className="stack">
      <PageHeader
        title="Profile"
        meta={
          <span>
            Ein Profil ist eine Folge von fio-Messungen. Beim Ausführen laufen die Schritte nacheinander, jeder
            Schritt gleichzeitig auf allen gewählten Hosts.
          </span>
        }
        actions={
          <Link className="btn" to="/profiles/new">
            <Icon name="plus" /> Neues Profil
          </Link>
        }
      />

      {err && (
        <div className="callout err">
          <Icon name="alert" />
          <div className="body">{err}</div>
        </div>
      )}

      <section className="panel">
        <div className="panel-head">
          <h2>Alle Profile</h2>
          <span className="sub">{profiles.length}</span>
          <div className="actions">
            <div className="search">
              <Icon name="search" size={14} />
              <input placeholder="Profil suchen…" value={q} onChange={(e) => setQ(e.target.value)} />
            </div>
          </div>
        </div>

        {loading ? (
          <div className="empty">Lädt…</div>
        ) : profiles.length === 0 ? (
          <Empty
            title="Noch keine Profile"
            action={
              <Link className="btn" to="/profiles/new">
                <Icon name="plus" /> Erstes Profil anlegen
              </Link>
            }
          >
            Lege fest, welche Lastmuster gemessen werden sollen, z. B. 4K Random Read und 1M sequenziell.
          </Empty>
        ) : shown.length === 0 ? (
          <Empty title="Kein Treffer" compact>
            Kein Profil passt zu „{q}“.
          </Empty>
        ) : (
          <div className="table-wrap">
            <table className="data">
              <thead>
                <tr>
                  <th>Profil</th>
                  <th>Ablauf</th>
                  <th className="num">Dauer</th>
                  <th>Geändert</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {shown.map((p) => (
                  <tr key={p.id} className="click" onClick={() => nav(`/profiles/${p.id}`)}>
                    <td style={{ minWidth: 200 }}>
                      <div className="cell-title">{p.name}</div>
                      {p.description && <div className="cell-sub">{p.description}</div>}
                    </td>
                    <td>
                      <div className="row" style={{ gap: 4, flexWrap: "wrap" }}>
                        {p.steps.slice(0, 4).map((s, i) => (
                          <span className="tag" key={i} title={s.spec.name}>
                            {workloadLabel(s.spec)} · {s.spec.blockSize}
                          </span>
                        ))}
                        {p.steps.length > 4 && <span className="tag">+{p.steps.length - 4} weitere</span>}
                      </div>
                    </td>
                    <td className="num">{fmtDuration(profileSeconds(p.steps))}</td>
                    <td className="muted when" title={fmtDateTime(p.updatedAt)}>
                      {fmtRelative(p.updatedAt)}
                    </td>
                    <td onClick={(e) => e.stopPropagation()} style={{ width: 1 }}>
                      <div className="row" style={{ gap: 4, justifyContent: "flex-end" }}>
                        <button
                          className="btn secondary sm"
                          onClick={() => setFire(p)}
                          disabled={onlineHosts === 0}
                          title={onlineHosts === 0 ? "Kein ausführbarer Host online" : "Profil ausführen"}
                        >
                          <Icon name="play" size={12} /> Ausführen
                        </button>
                        <button className="btn ghost icon sm" onClick={() => nav(`/profiles/${p.id}`)} title="Bearbeiten">
                          <Icon name="edit" />
                        </button>
                        <button className="btn ghost icon sm" onClick={() => duplicate(p)} title="Duplizieren">
                          <Icon name="copy" />
                        </button>
                        <button className="btn ghost icon sm" onClick={() => setDel(p)} title="Löschen">
                          <Icon name="trash" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {fire && <RunProfileDialog profile={fire} onClose={() => setFire(null)} />}
      {del && (
        <ConfirmDialog
          title="Profil löschen?"
          confirmLabel="Endgültig löschen"
          danger
          busy={busy}
          onCancel={() => setDel(null)}
          onConfirm={remove}
        >
          „{del.name}“ wird gelöscht. Bereits gemessene Ergebnisse bleiben erhalten.
        </ConfirmDialog>
      )}
    </div>
  );
}
