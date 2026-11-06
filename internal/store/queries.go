package store

import (
	"encoding/json"

	"github.com/storagebench/storagebench/internal/protocol"
)

// NodeRecord is a persisted node row for the API.
type NodeRecord struct {
	AgentID      string                `json:"agentId"`
	Hostname     string                `json:"hostname"`
	System       protocol.SystemInfo   `json:"system"`
	Capabilities protocol.Capabilities `json:"capabilities"`
	LastSeen     string                `json:"lastSeen"`
}

// ListNodes returns all known nodes.
func (s *Store) ListNodes() ([]NodeRecord, error) {
	rows, err := s.db.Query(`SELECT agent_id, hostname, system_info, capabilities, last_seen FROM node ORDER BY agent_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NodeRecord
	for rows.Next() {
		var rec NodeRecord
		var sysJSON, capJSON string
		if err := rows.Scan(&rec.AgentID, &rec.Hostname, &sysJSON, &capJSON, &rec.LastSeen); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(sysJSON), &rec.System)
		_ = json.Unmarshal([]byte(capJSON), &rec.Capabilities)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// RunSummary is a compact run row for list views.
type RunSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

// ListRuns returns recent runs, newest first, limited to limit rows.
func (s *Store) ListRuns(limit int) ([]RunSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, name, status, started_at, ended_at
		FROM test_run
		ORDER BY (CASE WHEN started_at = '' THEN id ELSE started_at END) DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RunSummary
	for rows.Next() {
		var r RunSummary
		if err := rows.Scan(&r.ID, &r.Name, &r.Status, &r.StartedAt, &r.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
