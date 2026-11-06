package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Batch is one execution of a profile against a set of hosts. It groups the runs
// (one per profile step) so results can be viewed together.
type Batch struct {
	ID         string   `json:"id"`
	ProfileID  string   `json:"profileId"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	AgentIDs   []string `json:"agentIds"`
	TotalSteps int      `json:"totalSteps"`
	StepIndex  int      `json:"stepIndex"`
	StartedAt  string   `json:"startedAt"`
	EndedAt    string   `json:"endedAt"`
}

// BatchDetail is a batch plus the runs that belong to it (one per step).
type BatchDetail struct {
	Batch
	ProfileName string          `json:"profileName"`
	Runs        []BatchRunEntry `json:"runs"`
}

// BatchRunEntry is a compact view of one run within a batch.
type BatchRunEntry struct {
	RunID     string `json:"runId"`
	Step      int    `json:"step"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

// CreateBatch inserts a new batch row and returns it with id/timestamps set.
func (s *Store) CreateBatch(profileID, name string, agentIDs []string, totalSteps int) (Batch, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	b := Batch{
		ID:         newID("batch"),
		ProfileID:  profileID,
		Name:       name,
		Status:     "running",
		AgentIDs:   agentIDs,
		TotalSteps: totalSteps,
		StepIndex:  0,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	agents, _ := json.Marshal(agentIDs)
	_, err := s.db.Exec(`
		INSERT INTO test_batch (id, profile_id, name, status, agent_ids, total_steps, step_index, started_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
		b.ID, nullIfEmpty(profileID), name, b.Status, string(agents), totalSteps, b.StartedAt)
	if err != nil {
		return Batch{}, err
	}
	return b, nil
}

// SetBatchStep records which step a batch is currently executing.
func (s *Store) SetBatchStep(batchID string, stepIndex int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`UPDATE test_batch SET step_index=? WHERE id=?`, stepIndex, batchID)
	return err
}

// SetBatchStatus updates a batch's status and, for terminal states, end time.
func (s *Store) SetBatchStatus(batchID, status string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	switch status {
	case "completed", "failed", "aborted":
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.db.Exec(`UPDATE test_batch SET status=?, ended_at=? WHERE id=?`, status, now, batchID)
		return err
	default:
		_, err := s.db.Exec(`UPDATE test_batch SET status=? WHERE id=?`, status, batchID)
		return err
	}
}

// CreateBatchRun inserts a run that belongs to a batch at the given step index.
func (s *Store) CreateBatchRun(runID, name string, resolvedConfig json.RawMessage, profileID, batchID string, step int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	cfg := string(resolvedConfig)
	if cfg == "" {
		cfg = "{}"
	}
	_, err := s.db.Exec(`
		INSERT INTO test_run (id, name, status, resolved_config, profile_id, batch_id, batch_step, started_at)
		VALUES (?, ?, 'created', ?, ?, ?, ?, ?)`,
		runID, name, cfg, nullIfEmpty(profileID), batchID, step, now)
	return err
}

// GetBatch returns a batch and its runs, ok=false if not found.
func (s *Store) GetBatch(batchID string) (BatchDetail, bool, error) {
	var d BatchDetail
	var agents string
	var profileID sql.NullString
	err := s.db.QueryRow(`
		SELECT b.id, COALESCE(b.profile_id,''), b.name, b.status, b.agent_ids,
		       b.total_steps, b.step_index, b.started_at, b.ended_at,
		       COALESCE(p.name, '')
		FROM test_batch b
		LEFT JOIN test_profile p ON p.id = b.profile_id
		WHERE b.id=?`, batchID).Scan(
		&d.ID, &profileID, &d.Name, &d.Status, &agents,
		&d.TotalSteps, &d.StepIndex, &d.StartedAt, &d.EndedAt, &d.ProfileName)
	if err == sql.ErrNoRows {
		return BatchDetail{}, false, nil
	}
	if err != nil {
		return BatchDetail{}, false, err
	}
	d.ProfileID = profileID.String
	_ = json.Unmarshal([]byte(agents), &d.AgentIDs)

	rows, err := s.db.Query(`
		SELECT id, batch_step, name, status, started_at, ended_at
		FROM test_run WHERE batch_id=? ORDER BY batch_step`, batchID)
	if err != nil {
		return d, true, err
	}
	defer rows.Close()
	for rows.Next() {
		var e BatchRunEntry
		if err := rows.Scan(&e.RunID, &e.Step, &e.Name, &e.Status, &e.StartedAt, &e.EndedAt); err != nil {
			return d, true, err
		}
		d.Runs = append(d.Runs, e)
	}
	return d, true, rows.Err()
}

// ListBatches returns recent batches, newest first.
func (s *Store) ListBatches(limit int) ([]Batch, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, COALESCE(profile_id,''), name, status, agent_ids, total_steps, step_index, started_at, ended_at
		FROM test_batch
		ORDER BY (CASE WHEN started_at='' THEN id ELSE started_at END) DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Batch
	for rows.Next() {
		var b Batch
		var agents string
		if err := rows.Scan(&b.ID, &b.ProfileID, &b.Name, &b.Status, &agents, &b.TotalSteps, &b.StepIndex, &b.StartedAt, &b.EndedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(agents), &b.AgentIDs)
		out = append(out, b)
	}
	return out, rows.Err()
}

// BatchStepRunStatuses returns the statuses of all runs for a given batch step.
// Used by the executor to know when a step is complete before advancing.
func (s *Store) BatchStepRunStatuses(batchID string, step int) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT status FROM test_run WHERE batch_id=? AND batch_step=?`, batchID, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// nullIfEmpty returns a NULL-able string: empty -> NULL so FKs stay clean.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
