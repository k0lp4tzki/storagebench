import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { ProfileStep } from "../api/types";
import { SpecForm, defaultSpec } from "../components/SpecForm";

export function ProfileEditor() {
  const { id } = useParams();
  const nav = useNavigate();
  const isNew = !id;

  const [name, setName] = useState("Neues Profil");
  const [description, setDescription] = useState("");
  const [steps, setSteps] = useState<ProfileStep[]>([{ spec: defaultSpec("Schritt 1") }]);
  const [openIdx, setOpenIdx] = useState(0);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (isNew) return;
    api
      .profile(id!)
      .then((p) => {
        setName(p.name);
        setDescription(p.description);
        setSteps(p.steps.length ? p.steps : [{ spec: defaultSpec("Schritt 1") }]);
      })
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false));
  }, [id, isNew]);

  const addStep = () => {
    setSteps((s) => [...s, { spec: defaultSpec(`Schritt ${s.length + 1}`) }]);
    setOpenIdx(steps.length);
  };
  const removeStep = (i: number) =>
    setSteps((s) => s.filter((_, idx) => idx !== i));
  const move = (i: number, dir: -1 | 1) =>
    setSteps((s) => {
      const j = i + dir;
      if (j < 0 || j >= s.length) return s;
      const next = [...s];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  const updateStep = (i: number, step: ProfileStep) =>
    setSteps((s) => s.map((old, idx) => (idx === i ? step : old)));

  async function save() {
    setSaving(true);
    setErr(null);
    try {
      const payload = { name, description, steps };
      if (isNew) {
        const created = await api.createProfile(payload);
        nav(`/profiles/${created.id}`);
      } else {
        await api.updateProfile(id!, { id: id!, ...payload });
        nav("/profiles");
      }
    } catch (e) {
      setErr(String(e));
      setSaving(false);
    }
  }

  if (loading) return <div className="card empty">Lädt…</div>;

  return (
    <div style={{ maxWidth: 820 }}>
      <div className="section-head">
        <h2>{isNew ? "Neues Profil" : "Profil bearbeiten"}</h2>
        <div className="toolbar">
          <button className="btn secondary" onClick={() => nav("/profiles")}>
            Abbrechen
          </button>
          <button className="btn" onClick={save} disabled={saving}>
            {saving ? "Speichern…" : "Speichern"}
          </button>
        </div>
      </div>

      {err && <div className="error" style={{ marginBottom: 14 }}>{err}</div>}

      <div className="card" style={{ marginBottom: 20 }}>
        <div className="row">
          <div className="field" style={{ flex: 2 }}>
            <label>Profilname</label>
            <input value={name} onChange={(e) => setName(e.target.value)} />
          </div>
        </div>
        <div className="field" style={{ marginBottom: 0 }}>
          <label>Beschreibung (optional)</label>
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="z. B. SSD-Baseline für neue Hosts"
          />
        </div>
      </div>

      <div className="section-head">
        <h2>Schritte ({steps.length})</h2>
        <button className="btn sm" onClick={addStep}>
          + Schritt
        </button>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {steps.map((step, i) => {
          const open = openIdx === i;
          return (
            <div className="card" key={i} style={{ padding: 0 }}>
              <div
                className="toolbar"
                style={{ padding: "12px 16px", cursor: "pointer" }}
                onClick={() => setOpenIdx(open ? -1 : i)}
              >
                <span className="badge accent">{i + 1}</span>
                <div style={{ fontWeight: 600 }}>{step.spec.name}</div>
                <span className="faint" style={{ fontSize: 12 }}>
                  {step.spec.workload} · {step.spec.blockSize} · QD{step.spec.ioDepth} ·{" "}
                  {step.spec.runtimeSeconds}s
                </span>
                <div className="spacer" />
                <button
                  className="btn ghost sm"
                  onClick={(e) => {
                    e.stopPropagation();
                    move(i, -1);
                  }}
                  disabled={i === 0}
                >
                  ↑
                </button>
                <button
                  className="btn ghost sm"
                  onClick={(e) => {
                    e.stopPropagation();
                    move(i, 1);
                  }}
                  disabled={i === steps.length - 1}
                >
                  ↓
                </button>
                <button
                  className="btn ghost sm"
                  onClick={(e) => {
                    e.stopPropagation();
                    removeStep(i);
                  }}
                  disabled={steps.length === 1}
                >
                  ✕
                </button>
              </div>
              {open && (
                <div style={{ padding: "4px 16px 18px", borderTop: "1px solid var(--border)" }}>
                  <SpecForm spec={step.spec} onChange={(spec) => updateStep(i, { spec })} />
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
