import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { useLive } from "../state/LiveProvider";
import type { Profile } from "../api/types";
import { Modal } from "../components/Modal";
import { fmtTime } from "../format";

export function Profiles() {
  const nav = useNavigate();
  const { agents } = useLive();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // Fire dialog state
  const [fireProfile, setFireProfile] = useState<Profile | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [firing, setFiring] = useState(false);
  const [fireErr, setFireErr] = useState<string | null>(null);

  const load = () =>
    api
      .profiles()
      .then((p) => setProfiles(p ?? []))
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false));

  useEffect(() => {
    load();
  }, []);

  async function remove(id: string) {
    if (!confirm("Profil wirklich löschen?")) return;
    try {
      await api.deleteProfile(id);
      load();
    } catch (e) {
      setErr(String(e));
    }
  }

  function openFire(p: Profile) {
    setFireProfile(p);
    setFireErr(null);
    setSelected(new Set(agents.filter((a) => a.status === "online").map((a) => a.agentId)));
  }

  async function fire() {
    if (!fireProfile || selected.size === 0) return;
    setFiring(true);
    setFireErr(null);
    try {
      const { batchId } = await api.startBatch({
        profileId: fireProfile.id,
        agentIds: [...selected],
      });
      setFireProfile(null);
      nav(`/batches/${batchId}`);
    } catch (e) {
      setFireErr(String(e));
      setFiring(false);
    }
  }

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });

  return (
    <div>
      <div className="section-head">
        <h2>Test-Profile</h2>
        <Link className="btn" to="/profiles/new">
          + Neues Profil
        </Link>
      </div>
      <p className="muted" style={{ marginTop: -6, marginBottom: 18 }}>
        Ein Profil bündelt mehrere Tests. Du kannst es gegen einen oder mehrere
        Hosts abfeuern – die Schritte laufen nacheinander, pro Schritt alle Hosts
        gleichzeitig.
      </p>

      {err && <div className="error">{err}</div>}

      {loading ? (
        <div className="card empty">Lädt…</div>
      ) : profiles.length === 0 ? (
        <div className="card empty">
          Noch keine Profile. Lege dein erstes Profil an.
        </div>
      ) : (
        <div className="grid cols-2">
          {profiles.map((p) => (
            <div className="card" key={p.id}>
              <div className="toolbar" style={{ alignItems: "start" }}>
                <div>
                  <div style={{ fontWeight: 650, fontSize: 15 }}>{p.name}</div>
                  <div className="faint" style={{ fontSize: 12, marginTop: 2 }}>
                    {p.steps.length} Schritt(e) · aktualisiert {fmtTime(p.updatedAt)}
                  </div>
                </div>
                <div className="spacer" />
                <span className="badge accent">{p.steps.length} ×</span>
              </div>

              {p.description && (
                <p className="muted" style={{ fontSize: 13, margin: "10px 0 0" }}>
                  {p.description}
                </p>
              )}

              <div className="chiprow" style={{ marginTop: 12 }}>
                {p.steps.slice(0, 5).map((s, i) => (
                  <span className="badge" key={i}>
                    {s.spec.workload} · {s.spec.blockSize}
                  </span>
                ))}
                {p.steps.length > 5 && (
                  <span className="badge">+{p.steps.length - 5}</span>
                )}
              </div>

              <div className="toolbar" style={{ marginTop: 16 }}>
                <button className="btn sm" onClick={() => openFire(p)}>
                  ▶ Ausführen
                </button>
                <Link className="btn sm secondary" to={`/profiles/${p.id}`}>
                  Bearbeiten
                </Link>
                <div className="spacer" />
                <button className="btn sm ghost" onClick={() => remove(p.id)}>
                  Löschen
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {fireProfile && (
        <Modal
          title={`"${fireProfile.name}" ausführen`}
          onClose={() => setFireProfile(null)}
          footer={
            <>
              <button className="btn secondary" onClick={() => setFireProfile(null)}>
                Abbrechen
              </button>
              <button
                className="btn"
                disabled={selected.size === 0 || firing}
                onClick={fire}
              >
                {firing ? "Starte…" : `Auf ${selected.size} Host(s) starten`}
              </button>
            </>
          }
        >
          <p className="muted" style={{ marginTop: 0 }}>
            Ziel-Hosts auswählen. Die {fireProfile.steps.length} Schritte laufen
            nacheinander, pro Schritt gleichzeitig auf allen gewählten Hosts.
          </p>
          {agents.length === 0 && <div className="empty">Keine Hosts verbunden.</div>}
          {agents.map((a) => (
            <label className="checkbox" key={a.agentId} style={{ padding: "6px 0" }}>
              <input
                type="checkbox"
                checked={selected.has(a.agentId)}
                onChange={() => toggle(a.agentId)}
                disabled={a.status === "offline"}
              />
              {a.system.hostname}
              <span className="faint" style={{ fontSize: 12 }}>
                ({a.status}
                {a.capabilities.fio ? "" : ", kein fio"})
              </span>
            </label>
          ))}
          {fireErr && <div className="error" style={{ marginTop: 10 }}>{fireErr}</div>}
        </Modal>
      )}
    </div>
  );
}
