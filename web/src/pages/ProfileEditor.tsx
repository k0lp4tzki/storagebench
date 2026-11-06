import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { Profile, ProfileStep } from "../api/types";
import { SpecForm, defaultSpec, needsConsent, validateSpec } from "../components/SpecForm";
import { PageHeader } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { ConfirmDialog } from "../components/Modal";
import { RunProfileDialog } from "../components/RunProfileDialog";
import { profileSeconds, specSummary, stepSeconds, workloadLabel } from "../domain";
import { fmtDuration } from "../format";

type Draft = { name: string; description: string; steps: ProfileStep[] };

const NEW_DRAFT = (): Draft => ({
  name: "Neues Profil",
  description: "",
  steps: [{ spec: defaultSpec("Schritt 1") }],
});

const snapshot = (d: Draft) => JSON.stringify(d);

export function ProfileEditor() {
  const { id } = useParams();
  const nav = useNavigate();
  const isNew = !id;

  const [draft, setDraft] = useState<Draft>(NEW_DRAFT);
  const [saved, setSaved] = useState<string>(() => (isNew ? "" : snapshot(NEW_DRAFT())));
  const [savedProfile, setSavedProfile] = useState<Profile | null>(null);
  const [sel, setSel] = useState(0);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<Date | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [running, setRunning] = useState(false);

  useEffect(() => {
    if (isNew) {
      const d = NEW_DRAFT();
      setDraft(d);
      setSaved("");
      setSel(0);
      return;
    }
    setLoading(true);
    api
      .profile(id!)
      .then((p) => {
        const d: Draft = {
          name: p.name,
          description: p.description,
          steps: p.steps.length ? p.steps : [{ spec: defaultSpec("Schritt 1") }],
        };
        setDraft(d);
        setSaved(snapshot(d));
        setSavedProfile(p);
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  }, [id, isNew]);

  const dirty = snapshot(draft) !== saved;
  const errors = useMemo(() => draft.steps.map((s) => validateSpec(s.spec)), [draft.steps]);
  const errorCount = errors.reduce((a, e) => a + Object.keys(e).length, 0) + (draft.name.trim() ? 0 : 1);
  const total = profileSeconds(draft.steps);
  const step = draft.steps[Math.min(sel, draft.steps.length - 1)];

  // Warn before closing the tab with unsaved changes.
  useEffect(() => {
    if (!dirty) return;
    const h = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [dirty]);

  const setSteps = (fn: (s: ProfileStep[]) => ProfileStep[]) => setDraft((d) => ({ ...d, steps: fn(d.steps) }));

  const addStep = () => {
    setSteps((s) => {
      // New steps inherit the target of the last step: usually the same disk.
      const prev = s[s.length - 1]?.spec;
      const spec = defaultSpec(`Schritt ${s.length + 1}`);
      if (prev) {
        spec.targetKind = prev.targetKind;
        spec.targetPath = prev.targetPath;
        spec.fileSizeMiB = prev.fileSizeMiB;
      }
      return [...s, { spec }];
    });
    setSel(draft.steps.length);
  };
  const duplicate = (i: number) => {
    setSteps((s) => {
      const copy = { spec: { ...s[i].spec, name: `${s[i].spec.name} (Kopie)` } };
      return [...s.slice(0, i + 1), copy, ...s.slice(i + 1)];
    });
    setSel(i + 1);
  };
  const remove = (i: number) => {
    setSteps((s) => s.filter((_, idx) => idx !== i));
    setSel((cur) => Math.max(0, cur >= i ? cur - 1 : cur));
  };
  const move = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= draft.steps.length) return;
    setSteps((s) => {
      const next = [...s];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
    setSel(j);
  };

  const save = useCallback(async () => {
    if (errorCount > 0 || saving) return;
    setSaving(true);
    setErr(null);
    try {
      if (isNew) {
        const created = await api.createProfile(draft);
        setSaved(snapshot(draft));
        setSavedProfile(created);
        setSavedAt(new Date());
        nav(`/profiles/${created.id}`, { replace: true });
      } else {
        const updated = await api.updateProfile(id!, { id: id!, ...draft });
        setSaved(snapshot(draft));
        setSavedProfile(updated ?? { ...(savedProfile as Profile), ...draft });
        setSavedAt(new Date());
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }, [draft, errorCount, id, isNew, nav, saving, savedProfile]);

  // Ctrl/Cmd+S saves.
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        save();
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [save]);

  const leave = () => (dirty ? setConfirmLeave(true) : nav("/profiles"));

  if (loading) return <div className="panel empty">Profil wird geladen…</div>;

  return (
    <div>
      <PageHeader
        crumbs={[{ to: "/profiles", label: "Profile" }]}
        title={draft.name.trim() || "Unbenanntes Profil"}
        meta={
          <>
            <span>
              {draft.steps.length} {draft.steps.length === 1 ? "Schritt" : "Schritte"}
            </span>
            <span className="sep">|</span>
            <span className="row" style={{ gap: 5 }}>
              <Icon name="clock" size={14} /> ca. {fmtDuration(total)} pro Host
            </span>
            {isNew && (
              <>
                <span className="sep">|</span>
                <span className="badge plain">Entwurf, noch nicht gespeichert</span>
              </>
            )}
          </>
        }
        actions={
          !isNew && (
            <button
              className="btn secondary"
              onClick={() => setRunning(true)}
              disabled={dirty || !savedProfile}
              title={dirty ? "Erst speichern, dann ausführen" : "Profil ausführen"}
            >
              <Icon name="play" size={13} /> Ausführen
            </button>
          )
        }
      />

      <div className="panel" style={{ marginBottom: 20 }}>
        <div className="panel-body form-grid">
          <div className={`field span-2 ${draft.name.trim() ? "" : "invalid"}`}>
            <label htmlFor="pname">Profilname</label>
            <input id="pname" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
            {!draft.name.trim() && <span className="field-err">Name fehlt</span>}
          </div>
          <div className="field span-2">
            <label htmlFor="pdesc">Beschreibung</label>
            <input
              id="pdesc"
              value={draft.description}
              onChange={(e) => setDraft({ ...draft, description: e.target.value })}
              placeholder="optional, z. B. SSD-Baseline für neue Hosts"
            />
          </div>
        </div>
      </div>

      <div className="editor">
        {/* Step sequence */}
        <section className="panel" style={{ position: "sticky", top: 16 }}>
          <div className="panel-head">
            <h2>Ablauf</h2>
            <span className="sub">nacheinander</span>
            <div className="actions">
              <button className="btn secondary sm" onClick={addStep}>
                <Icon name="plus" size={14} /> Schritt
              </button>
            </div>
          </div>
          <ol className="steps-list">
            {draft.steps.map((s, i) => {
              const bad = Object.keys(errors[i]).length > 0;
              const warn = !bad && needsConsent(s.spec);
              return (
                <li key={i}>
                  <button className={`step-item ${i === sel ? "on" : ""}`} onClick={() => setSel(i)}>
                    <span className="step-no">{i + 1}</span>
                    <span className="grow">
                      <div className="s-title ellipsis">{s.spec.name || "Unbenannt"}</div>
                      <div className="s-sub ellipsis">
                        {workloadLabel(s.spec)} · {specSummary(s.spec)}
                      </div>
                    </span>
                    {bad && (
                      <span className="flag err" title="Eingaben unvollständig">
                        <Icon name="alert" size={15} />
                      </span>
                    )}
                    {warn && (
                      <span className="flag warn" title="Destruktiver Schreibtest nicht bestätigt">
                        <Icon name="alert" size={15} />
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ol>
          <div className="steps-foot">
            <span>Gesamtdauer</span>
            <span className="mono">{fmtDuration(total)}</span>
          </div>
        </section>

        {/* Selected step */}
        {step && (
          <section className="panel">
            <div className="panel-head">
              <span className="step-no" style={{ background: "var(--accent)", color: "var(--on-accent)" }}>
                {sel + 1}
              </span>
              <h2 className="ellipsis">{step.spec.name || "Unbenannt"}</h2>
              <span className="sub mono">{fmtDuration(stepSeconds(step.spec))}</span>
              <div className="actions">
                <button className="btn ghost icon sm" onClick={() => move(sel, -1)} disabled={sel === 0} title="Nach oben">
                  <Icon name="up" />
                </button>
                <button
                  className="btn ghost icon sm"
                  onClick={() => move(sel, 1)}
                  disabled={sel === draft.steps.length - 1}
                  title="Nach unten"
                >
                  <Icon name="down" />
                </button>
                <button className="btn ghost icon sm" onClick={() => duplicate(sel)} title="Schritt duplizieren">
                  <Icon name="copy" />
                </button>
                <button
                  className="btn ghost icon sm"
                  onClick={() => remove(sel)}
                  disabled={draft.steps.length === 1}
                  title={draft.steps.length === 1 ? "Ein Profil braucht mindestens einen Schritt" : "Schritt entfernen"}
                >
                  <Icon name="trash" />
                </button>
              </div>
            </div>
            <div className="panel-body">
              <SpecForm
                spec={step.spec}
                errors={errors[sel]}
                onChange={(spec) => setSteps((s) => s.map((old, idx) => (idx === sel ? { spec } : old)))}
              />
            </div>
          </section>
        )}
      </div>

      {/* Save bar: state of the draft is always visible next to the action */}
      <div className="savebar" role="region" aria-label="Speichern">
        {err ? (
          <span className="state err">
            <Icon name="alert" /> Speichern fehlgeschlagen: {err}
          </span>
        ) : errorCount > 0 ? (
          <span className="state err">
            <Icon name="alert" /> {errorCount} {errorCount === 1 ? "Eingabe muss" : "Eingaben müssen"} korrigiert werden
          </span>
        ) : dirty ? (
          <span className="state dirty">
            <span className="live-dot" style={{ background: "var(--warn)" }} /> Ungespeicherte Änderungen
          </span>
        ) : (
          <span className="state">
            <Icon name="check" />
            {savedAt
              ? `Gespeichert um ${savedAt.toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" })}`
              : "Alle Änderungen gespeichert"}
          </span>
        )}
        <span className="grow" />
        <button className="btn secondary" onClick={leave}>
          {dirty ? "Verwerfen" : "Zurück"}
        </button>
        <button className="btn" onClick={save} disabled={saving || errorCount > 0 || (!dirty && !isNew)}>
          {saving ? "Speichert…" : "Speichern"}
          <span className="kbd">Strg S</span>
        </button>
      </div>

      {confirmLeave && (
        <ConfirmDialog
          title="Änderungen verwerfen?"
          confirmLabel="Verwerfen"
          danger
          onCancel={() => setConfirmLeave(false)}
          onConfirm={() => {
            setSaved(snapshot(draft));
            nav("/profiles");
          }}
        >
          Die ungespeicherten Änderungen an „{draft.name}“ gehen verloren.
        </ConfirmDialog>
      )}
      {running && savedProfile && (
        <RunProfileDialog profile={{ ...savedProfile, ...draft }} onClose={() => setRunning(false)} />
      )}
    </div>
  );
}
