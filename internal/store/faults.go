package store

import "database/sql"

// FaultRecord is one fault-injection action correlated with a run's timeline.
// TSInjectMilli and TSRevertMilli are wall-clock milliseconds (UTC); a zero
// TSRevertMilli means the fault is still active (not yet reverted).
type FaultRecord struct {
	ID            int64  `json:"id"`
	RunID         string `json:"runId"`
	FaultID       string `json:"faultId"`
	AgentID       string `json:"agentId"`
	Kind          string `json:"kind"`
	Iface         string `json:"iface"`
	Detail        string `json:"detail"`
	TSInjectMilli int64  `json:"tsInjectMilli"`
	TSRevertMilli int64  `json:"tsRevertMilli"`
}

// RecordFaultInject inserts a new fault action with its inject timestamp and
// returns the row id. faultID is a controller-generated id used to pair the
// later revert. detail is a human-readable summary (e.g. "Latenz +100ms auf eth1").
func (s *Store) RecordFaultInject(runID, faultID, agentID, kind, iface, detail string, tsInjectMilli int64) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`
		INSERT INTO fault_action (run_id, fault_id, agent_id, kind, iface, detail, ts_inject, ts_revert)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		runID, faultID, agentID, kind, iface, detail, tsInjectMilli)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecordFaultRevert stamps the revert time on the matching active fault for a run
// and interface (the most recent one still active).
func (s *Store) RecordFaultRevert(runID, iface string, tsRevertMilli int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`
		UPDATE fault_action SET ts_revert = ?
		WHERE id = (
			SELECT id FROM fault_action
			WHERE run_id = ? AND iface = ? AND ts_revert = 0
			ORDER BY ts_inject DESC LIMIT 1
		)`, tsRevertMilli, runID, iface)
	return err
}

// ListRunFaults returns all fault actions for a run, oldest first, for the
// timeline overlay.
func (s *Store) ListRunFaults(runID string) ([]FaultRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, run_id, fault_id, agent_id, kind, iface, detail, ts_inject, ts_revert
		FROM fault_action
		WHERE run_id = ?
		ORDER BY ts_inject`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FaultRecord
	for rows.Next() {
		var f FaultRecord
		var faultID, agentID, kind, iface, detail sql.NullString
		if err := rows.Scan(&f.ID, &f.RunID, &faultID, &agentID, &kind, &iface, &detail,
			&f.TSInjectMilli, &f.TSRevertMilli); err != nil {
			return nil, err
		}
		f.FaultID = faultID.String
		f.AgentID = agentID.String
		f.Kind = kind.String
		f.Iface = iface.String
		f.Detail = detail.String
		out = append(out, f)
	}
	return out, rows.Err()
}
