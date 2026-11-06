package store

import (
	"encoding/json"

	"github.com/storagebench/storagebench/internal/protocol"
)

// RunDetail is the full view of a single run for the results page.
type RunDetail struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Status         string          `json:"status"`
	StartedAt      string          `json:"startedAt"`
	EndedAt        string          `json:"endedAt"`
	ResolvedConfig json.RawMessage `json:"resolvedConfig"`
	Nodes          []RunNodeDetail `json:"nodes"`
}

// RunNodeDetail is one node's participation in a run, with its frozen system
// snapshot and final fio summary.
type RunNodeDetail struct {
	AgentID        string          `json:"agentId"`
	Hostname       string          `json:"hostname"`
	Phase          string          `json:"phase"`
	PhaseMessage   string          `json:"phaseMessage"`
	SystemSnapshot json.RawMessage `json:"systemSnapshot"`
	FioSummary     json.RawMessage `json:"fioSummary"`
}

// GetRun returns the full detail of one run, or ok=false if not found.
func (s *Store) GetRun(runID string) (RunDetail, bool, error) {
	var d RunDetail
	var cfg string
	err := s.db.QueryRow(`
		SELECT id, name, status, started_at, ended_at, resolved_config
		FROM test_run WHERE id = ?`, runID).Scan(
		&d.ID, &d.Name, &d.Status, &d.StartedAt, &d.EndedAt, &cfg)
	if err != nil {
		return RunDetail{}, false, nil //nolint:nilerr // not found is not an error here
	}
	d.ResolvedConfig = json.RawMessage(nonEmptyJSON(cfg))

	rows, err := s.db.Query(`
		SELECT n.agent_id, n.hostname, rn.phase, rn.phase_message, rn.system_snapshot, rn.fio_summary
		FROM run_node rn
		JOIN node n ON n.id = rn.node_id
		WHERE rn.run_id = ?
		ORDER BY n.agent_id`, runID)
	if err != nil {
		return d, true, err
	}
	defer rows.Close()

	for rows.Next() {
		var nd RunNodeDetail
		var snap, summary string
		if err := rows.Scan(&nd.AgentID, &nd.Hostname, &nd.Phase, &nd.PhaseMessage, &snap, &summary); err != nil {
			return d, true, err
		}
		nd.SystemSnapshot = json.RawMessage(nonEmptyJSON(snap))
		if summary != "" {
			nd.FioSummary = json.RawMessage(summary)
		}
		d.Nodes = append(d.Nodes, nd)
	}
	return d, true, rows.Err()
}

// GetRunMetrics returns all metric samples for a run, grouped by agent id. This
// powers the results time-series charts.
func (s *Store) GetRunMetrics(runID string) (map[string][]protocol.MetricSample, error) {
	rows, err := s.db.Query(`
		SELECT n.agent_id,
		       m.ts_wall, m.ts_mono,
		       m.iops_r, m.iops_w, m.bw_r, m.bw_w,
		       m.lat_mean, m.lat_p95, m.lat_p99, m.qdepth,
		       m.cpu, m.mem, m.load1, m.disk_util, m.net_rx, m.net_tx
		FROM metric_sample m
		JOIN run_node rn ON rn.id = m.run_node_id
		JOIN node n ON n.id = rn.node_id
		WHERE rn.run_id = ?
		ORDER BY m.ts_wall`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]protocol.MetricSample)
	for rows.Next() {
		var agentID string
		var m protocol.MetricSample
		if err := rows.Scan(&agentID,
			&m.TSWallMilli, &m.MonoNanos,
			&m.IOPSRead, &m.IOPSWrite, &m.BWReadKBs, &m.BWWriteKBs,
			&m.LatMeanUs, &m.LatP95Us, &m.LatP99Us, &m.QueueDepth,
			&m.CPUPercent, &m.MemPercent, &m.LoadAvg1, &m.DiskUtilPct, &m.NetRxKBs, &m.NetTxKBs); err != nil {
			return nil, err
		}
		out[agentID] = append(out[agentID], m)
	}
	return out, rows.Err()
}

// GetRunEvents returns the events for a run, newest first, limited.
func (s *Store) GetRunEvents(runID string, limit int) ([]EventRecord, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.Query(`
		SELECT e.agent_id, e.ts_wall, e.severity, e.source, e.message
		FROM event e
		JOIN run_node rn ON rn.id = e.run_node_id
		WHERE rn.run_id = ?
		ORDER BY e.ts_wall DESC
		LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventRecord
	for rows.Next() {
		var ev EventRecord
		if err := rows.Scan(&ev.AgentID, &ev.TSWallMilli, &ev.Severity, &ev.Source, &ev.Message); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// EventRecord is a persisted event for the API.
type EventRecord struct {
	AgentID     string `json:"agentId"`
	TSWallMilli int64  `json:"tsWallMilli"`
	Severity    string `json:"severity"`
	Source      string `json:"source"`
	Message     string `json:"message"`
}

// GetRunAgents returns the agent ids participating in a run.
func (s *Store) GetRunAgents(runID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT n.agent_id FROM run_node rn
		JOIN node n ON n.id = rn.node_id
		WHERE rn.run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// nonEmptyJSON returns s if it is non-empty JSON, else "{}".
func nonEmptyJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}
