package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/storagebench/storagebench/internal/benchmark"
	"github.com/storagebench/storagebench/internal/store"
)

// BatchExecutor runs a profile against a set of hosts: for each profile step it
// starts one run across all selected hosts simultaneously, waits for that step's
// run to finish, then advances to the next step. Steps are sequential; hosts
// within a step run in parallel.
type BatchExecutor struct {
	hub         *Hub
	store       *store.Store
	broadcaster *Broadcaster
	log         *slog.Logger
}

// NewBatchExecutor constructs a BatchExecutor. broadcaster may be nil.
func NewBatchExecutor(hub *Hub, st *store.Store, broadcaster *Broadcaster, log *slog.Logger) *BatchExecutor {
	return &BatchExecutor{hub: hub, store: st, broadcaster: broadcaster, log: log.With("component", "batch")}
}

// publishProgress broadcasts a batch progress update if a broadcaster is wired.
func (e *BatchExecutor) publishProgress(batchID, status string, stepIndex, totalSteps int, runID string) {
	if e.broadcaster == nil {
		return
	}
	e.broadcaster.Publish(UITypeBatch, UIBatch{
		BatchID:    batchID,
		Status:     status,
		StepIndex:  stepIndex,
		TotalSteps: totalSteps,
		RunID:      runID,
	})
}

// StartBatchRequest is the input to launch a profile against hosts.
type StartBatchRequest struct {
	ProfileID string   `json:"profileId"`
	AgentIDs  []string `json:"agentIds"`
	// Name overrides the batch name; defaults to the profile name.
	Name string `json:"name,omitempty"`
}

// Start validates the request, creates the batch, and launches execution in the
// background. It returns the batch id immediately.
func (e *BatchExecutor) Start(req StartBatchRequest) (string, error) {
	if len(req.AgentIDs) == 0 {
		return "", fmt.Errorf("no target hosts selected")
	}
	prof, ok, err := e.store.GetProfile(req.ProfileID)
	if err != nil {
		return "", fmt.Errorf("load profile: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("profile %q not found", req.ProfileID)
	}
	if err := prof.Validate(); err != nil {
		return "", err
	}
	// Confirm all hosts are connected up front.
	for _, agentID := range req.AgentIDs {
		if _, ok := e.hub.get(agentID); !ok {
			return "", fmt.Errorf("host %q is not connected", agentID)
		}
	}

	name := req.Name
	if name == "" {
		name = prof.Name
	}
	batch, err := e.store.CreateBatch(prof.ID, name, req.AgentIDs, len(prof.Steps))
	if err != nil {
		return "", fmt.Errorf("create batch: %w", err)
	}

	go e.run(batch, prof, req.AgentIDs)
	return batch.ID, nil
}

// run executes the profile steps sequentially. It is the background worker.
func (e *BatchExecutor) run(batch store.Batch, prof store.Profile, agentIDs []string) {
	e.log.Info("batch started", "batch", batch.ID, "profile", prof.Name, "steps", len(prof.Steps), "hosts", len(agentIDs))
	ctx := context.Background()

	worst := "completed"
	for i, step := range prof.Steps {
		if err := e.store.SetBatchStep(batch.ID, i); err != nil {
			e.log.Warn("set batch step failed", "batch", batch.ID, "err", err)
		}

		runID, err := e.startStep(batch, prof, step, agentIDs, i)
		if err != nil {
			e.log.Warn("batch step start failed", "batch", batch.ID, "step", i, "err", err)
			worst = "failed"
			break
		}
		e.publishProgress(batch.ID, "running", i, len(prof.Steps), runID)

		// Wait for this step's run to reach a terminal status, then record the
		// worst outcome across steps.
		status := e.waitForRun(ctx, runID, step.Spec.RuntimeSeconds)
		if status == "aborted" {
			worst = "aborted"
			break
		}
		if status == "failed" && worst != "aborted" {
			worst = "failed"
		}
	}

	_ = e.store.SetBatchStatus(batch.ID, worst)
	e.publishProgress(batch.ID, worst, len(prof.Steps), len(prof.Steps), "")
	e.log.Info("batch finished", "batch", batch.ID, "status", worst)
}

// startStep creates a batch run for one step and assigns/starts it on all hosts.
func (e *BatchExecutor) startStep(batch store.Batch, prof store.Profile, step store.ProfileStep, agentIDs []string, stepIdx int) (string, error) {
	// Build the per-agent spec map: the same step spec for every selected host.
	specs := make(map[string]benchmark.Spec, len(agentIDs))
	for _, agentID := range agentIDs {
		specs[agentID] = step.Spec
	}
	if err := e.hub.validateSpecs(specs); err != nil {
		return "", err
	}

	runID := newRunID()
	resolved, err := json.Marshal(specs)
	if err != nil {
		return "", fmt.Errorf("marshal resolved config: %w", err)
	}
	runName := fmt.Sprintf("%s · Schritt %d/%d: %s", prof.Name, stepIdx+1, len(prof.Steps), step.Spec.Name)
	if err := e.store.CreateBatchRun(runID, runName, resolved, prof.ID, batch.ID, stepIdx); err != nil {
		return "", fmt.Errorf("create batch run: %w", err)
	}
	if err := e.hub.assignAndStart(e.store, runID, specs, 0); err != nil {
		return runID, err
	}
	return runID, nil
}

// waitForRun polls a run's status until it is terminal or a safety timeout
// elapses (runtime plus a generous margin). Returns the final status.
func (e *BatchExecutor) waitForRun(ctx context.Context, runID string, runtimeSeconds int) string {
	deadline := time.Now().Add(time.Duration(runtimeSeconds+600) * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "aborted"
		case <-ticker.C:
			d, ok, err := e.store.GetRun(runID)
			if err != nil || !ok {
				continue
			}
			switch d.Status {
			case "completed", "failed", "aborted":
				return d.Status
			}
			if time.Now().After(deadline) {
				e.log.Warn("batch run wait timed out", "run", runID)
				return "failed"
			}
		}
	}
}
