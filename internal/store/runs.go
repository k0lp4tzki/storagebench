package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/storagebench/storagebench/internal/protocol"
)

// CreateRun inserts a new test run with its resolved configuration frozen in.
// resolvedConfig is the exact benchmark spec (as JSON) that will be executed, so
// the run is fully reproducible.
func (s *Store) CreateRun(runID, name string, resolvedConfig json.RawMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	cfg := string(resolvedConfig)
	if cfg == "" {
		cfg = "{}"
	}
	_, err := s.db.Exec(`
		INSERT INTO test_run (id, name, status, resolved_config, started_at)
		VALUES (?, ?, 'created', ?, ?)
	`, runID, name, cfg, now)
	return err
}

// AddRunNode links a node to a run and freezes that node's current system
// snapshot. It resolves the node's internal id from its agent id.
func (s *Store) AddRunNode(runID, agentID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var nodeID int64
	var sysInfo string
	err := s.db.QueryRow(`SELECT id, system_info FROM node WHERE agent_id = ?`, agentID).Scan(&nodeID, &sysInfo)
	if err != nil {
		return fmt.Errorf("resolve node %q: %w", agentID, err)
	}
	if sysInfo == "" {
		sysInfo = "{}"
	}
	_, err = s.db.Exec(`
		INSERT INTO run_node (run_id, node_id, phase, system_snapshot)
		VALUES (?, ?, 'assigned', ?)
		ON CONFLICT(run_id, node_id) DO UPDATE SET
			phase='assigned', system_snapshot=excluded.system_snapshot
	`, runID, nodeID, sysInfo)
	return err
}

// SetRunStatus updates a run's status and, for terminal states, its end time.
func (s *Store) SetRunStatus(runID, status string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.setRunStatusLocked(runID, status)
}

func (s *Store) setRunStatusLocked(runID, status string) error {
	terminal := status == "completed" || status == "failed" || status == "aborted"
	if terminal {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.db.Exec(`UPDATE test_run SET status=?, ended_at=? WHERE id=?`, status, now, runID)
		return err
	}
	// Set started_at on first transition to running if not already set.
	if status == "running" {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.db.Exec(`
			UPDATE test_run SET status=?,
				started_at = CASE WHEN started_at='' OR started_at IS NULL THEN ? ELSE started_at END
			WHERE id=?`, status, now, runID)
		return err
	}
	_, err := s.db.Exec(`UPDATE test_run SET status=? WHERE id=?`, status, runID)
	return err
}

// OnTestResult stores the final fio summary for a run_node and advances the run
// toward completion. It implements part of the controller.TelemetrySink
// interface.
func (s *Store) OnTestResult(agentID string, res protocol.TestResult) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	runNodeID, ok := s.lookupRunNode(res.RunID, agentID)
	if !ok {
		return
	}
	summary, _ := json.Marshal(map[string]any{
		"exitCode":   res.ExitCode,
		"iopsRead":   res.IOPSRead,
		"iopsWrite":  res.IOPSWrite,
		"bwReadKBs":  res.BWReadKBs,
		"bwWriteKBs": res.BWWriteKBs,
		"latMeanUs":  res.LatMeanUs,
		"latP95Us":   res.LatP95Us,
		"latP99Us":   res.LatP99Us,
	})
	_, _ = s.db.Exec(`UPDATE run_node SET fio_summary=?, phase='completed' WHERE id=?`, string(summary), runNodeID)

	// If every run_node for this run has completed, mark the run completed.
	s.maybeCompleteRunLocked(res.RunID)
}

// maybeCompleteRunLocked finalizes a run once all its run_nodes have reached a
// terminal phase. The run's status reflects the worst outcome: aborted if any
// node aborted, else failed if any node failed, else completed. Caller must hold
// writeMu. COALESCE guards the NULL that SUM() returns over zero rows.
func (s *Store) maybeCompleteRunLocked(runID string) {
	var total, done, failed, aborted int
	err := s.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN phase IN ('completed','failed','aborted') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN phase = 'failed'  THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN phase = 'aborted' THEN 1 ELSE 0 END), 0)
		FROM run_node WHERE run_id=?`, runID).Scan(&total, &done, &failed, &aborted)
	if err != nil || total == 0 || done < total {
		return
	}
	status := "completed"
	switch {
	case aborted > 0:
		status = "aborted"
	case failed > 0:
		status = "failed"
	}
	_ = s.setRunStatusLocked(runID, status)
}
