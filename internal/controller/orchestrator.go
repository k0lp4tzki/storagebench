package controller

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/storagebench/storagebench/internal/benchmark"
	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/protocol"
)

// StartRunRequest is the input to StartRun: a name, the target agents, and a
// per-agent spec. Using one spec per agent allows individual load profiles per
// server; callers may pass the same spec for all for a uniform test.
type StartRunRequest struct {
	Name string `json:"name"`
	// Specs maps agentID -> benchmark spec. Every listed agent must be connected.
	Specs map[string]benchmark.Spec `json:"specs"`
	// AuthorizationSeconds bounds how long agents may continue the test if the
	// controller disconnects. Defaults to runtime + a margin when zero.
	AuthorizationSeconds int `json:"authorizationSeconds,omitempty"`
}

// RunStore is the persistence surface the orchestrator needs. The store
// implements it; depending on an interface keeps the orchestrator testable.
type RunStore interface {
	CreateRun(runID, name string, resolvedConfig json.RawMessage) error
	AddRunNode(runID, agentID string) error
	SetRunStatus(runID, status string) error
}

// StartRun validates the request, persists the run and its nodes, then assigns
// and starts the test on each target agent. It fails fast if any target agent is
// not connected, to avoid partially-started runs.
func (h *Hub) StartRun(st RunStore, req StartRunRequest) (string, error) {
	if len(req.Specs) == 0 {
		return "", fmt.Errorf("no target agents specified")
	}

	// Validate specs and confirm all agents are connected before changing state.
	for agentID, spec := range req.Specs {
		if _, ok := h.get(agentID); !ok {
			return "", fmt.Errorf("agent %q is not connected", agentID)
		}
		s := spec
		if err := s.Validate(); err != nil {
			return "", fmt.Errorf("invalid spec for agent %q: %w", agentID, err)
		}
		// Refuse if the agent already has a running test would be ideal, but the
		// agent enforces one-active-test and will reject start; we surface that
		// asynchronously via test_state.
	}

	runID := newRunID()

	// Freeze the resolved configuration (the full per-agent spec map).
	resolved, err := json.Marshal(req.Specs)
	if err != nil {
		return "", fmt.Errorf("marshal resolved config: %w", err)
	}
	if err := st.CreateRun(runID, req.Name, resolved); err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}

	// Determine authorization deadline.
	maxRuntime := 0
	for _, spec := range req.Specs {
		if spec.RuntimeSeconds > maxRuntime {
			maxRuntime = spec.RuntimeSeconds
		}
	}
	authSecs := req.AuthorizationSeconds
	if authSecs <= 0 {
		authSecs = maxRuntime + 300 // runtime + 5 min margin
	}
	authUntil := clock.NowWallMilli() + int64(authSecs)*1000

	// Assign to each agent, then start. Assign first so an agent has the job
	// persisted before being told to start.
	for agentID, spec := range req.Specs {
		if err := st.AddRunNode(runID, agentID); err != nil {
			_ = st.SetRunStatus(runID, "failed")
			return "", fmt.Errorf("add run node %q: %w", agentID, err)
		}
		specRaw, err := json.Marshal(spec)
		if err != nil {
			_ = st.SetRunStatus(runID, "failed")
			return "", fmt.Errorf("marshal spec for %q: %w", agentID, err)
		}
		assign := protocol.AssignTest{
			RunID:                runID,
			AuthorizedUntilMilli: authUntil,
			Spec:                 specRaw,
		}
		if err := h.SendControl(agentID, protocol.TypeAssignTest, assign); err != nil {
			_ = st.SetRunStatus(runID, "failed")
			return "", fmt.Errorf("assign to %q: %w", agentID, err)
		}
	}

	// Start each agent. A short settling delay lets assignments land first.
	time.Sleep(100 * time.Millisecond)
	for agentID := range req.Specs {
		if err := h.SendControl(agentID, protocol.TypeStart, protocol.Start{RunID: runID}); err != nil {
			// Best-effort: mark failed but report which agent.
			_ = st.SetRunStatus(runID, "failed")
			return runID, fmt.Errorf("start on %q: %w", agentID, err)
		}
	}

	_ = st.SetRunStatus(runID, "running")
	return runID, nil
}

// StopRun sends a graceful stop to all agents participating in a run.
func (h *Hub) StopRun(agentIDs []string, runID string) {
	for _, agentID := range agentIDs {
		_ = h.SendControl(agentID, protocol.TypeStop, protocol.Stop{RunID: runID})
	}
}

// AbortRun sends an abort to all agents participating in a run.
func (h *Hub) AbortRun(agentIDs []string, runID, reason string) {
	for _, agentID := range agentIDs {
		_ = h.SendControl(agentID, protocol.TypeAbort, protocol.Abort{RunID: runID, Reason: reason})
	}
}

// newRunID generates a sortable, unique run id: a UTC timestamp prefix plus
// random hex. The timestamp prefix makes ids naturally time-ordered.
func newRunID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("run-%s-%s", time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(b[:]))
}
