// Package store is the controller's persistence layer. It uses SQLite in WAL
// mode: for three nodes this is more than sufficient, needs no server, and
// keeps the whole deployment to a single file that is trivial to back up.
//
// The schema freezes each run's resolved configuration and per-node system
// information so results are always reproducible — a result without its test
// conditions is considered worthless.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/storagebench/storagebench/internal/protocol"
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB

	// writeMu serializes writes. SQLite allows a single writer; a mutex gives us
	// clear, contention-free batching from multiple agent goroutines.
	writeMu sync.Mutex
}

// Open opens (and if necessary creates) the database at path and applies the
// schema. Parent directories are created as needed.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	// Pragmas: WAL for concurrent readers with one writer; busy_timeout so a
	// brief lock contention retries instead of erroring; foreign_keys on.
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite with a single connection avoids "database is locked" surprises for
	// writes while still serving reads from the same connection pool entry.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrate creates the schema if it does not exist. It is idempotent.
func (s *Store) migrate() error {
	_, err := s.db.Exec(schemaSQL)
	return err
}

// --- Node registration ---

// UpsertNode records or updates a node's identity and capabilities.
func (s *Store) UpsertNode(agentID string, h protocol.Hello) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	sysJSON, _ := json.Marshal(h.System)
	capJSON, _ := json.Marshal(h.Capabilities)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO node (agent_id, hostname, system_info, capabilities, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			hostname=excluded.hostname,
			system_info=excluded.system_info,
			capabilities=excluded.capabilities,
			last_seen=excluded.last_seen
	`, agentID, h.System.Hostname, string(sysJSON), string(capJSON), now)
	return err
}

// --- TelemetrySink implementation (invoked by the hub) ---

// IngestMetrics persists a batch of samples for the run_node identified by
// (runID, agentID). Unknown run/agent combinations are ignored (the run may not
// have been created yet in a degraded path); this keeps ingestion robust.
func (s *Store) IngestMetrics(agentID string, batch protocol.MetricBatch) {
	if len(batch.Samples) == 0 {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	runNodeID, ok := s.lookupRunNode(batch.RunID, agentID)
	if !ok {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO metric_sample
			(run_node_id, ts_wall, ts_mono, iops_r, iops_w, bw_r, bw_w, lat_mean, lat_p95, lat_p99, qdepth, cpu, mem, load1, disk_util, net_rx, net_tx)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return
	}
	defer stmt.Close()

	for _, m := range batch.Samples {
		if _, err := stmt.Exec(runNodeID, m.TSWallMilli, m.MonoNanos,
			m.IOPSRead, m.IOPSWrite, m.BWReadKBs, m.BWWriteKBs,
			m.LatMeanUs, m.LatP95Us, m.LatP99Us, m.QueueDepth,
			m.CPUPercent, m.MemPercent, m.LoadAvg1, m.DiskUtilPct, m.NetRxKBs, m.NetTxKBs); err != nil {
			return
		}
	}
	_ = tx.Commit()
}

// IngestEvent persists a discrete event.
func (s *Store) IngestEvent(agentID string, ev protocol.Event) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var runNodeID sql.NullInt64
	if ev.RunID != "" {
		if id, ok := s.lookupRunNode(ev.RunID, agentID); ok {
			runNodeID = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	_, _ = s.db.Exec(`
		INSERT INTO event (run_node_id, agent_id, ts_wall, ts_mono, severity, source, message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, runNodeID, agentID, ev.TSWallMilli, ev.MonoNanos, string(ev.Severity), ev.Source, ev.Message)
}

// OnAgentHello records an agent registration.
func (s *Store) OnAgentHello(agentID string, h protocol.Hello) {
	if err := s.UpsertNode(agentID, h); err != nil {
		// Registration failures are non-fatal for the live connection; log via
		// the caller's logger is not available here, so we swallow and rely on
		// the next hello to retry.
		_ = err
	}
}

// OnTestState updates the run_node phase for a lifecycle transition.
func (s *Store) OnTestState(agentID string, st protocol.TestState) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	runNodeID, ok := s.lookupRunNode(st.RunID, agentID)
	if !ok {
		return
	}
	_, _ = s.db.Exec(`UPDATE run_node SET phase=?, phase_message=? WHERE id=?`,
		string(st.Phase), st.Message, runNodeID)
}

// lookupRunNode resolves (runID, agentID) to a run_node row id. Caller must hold
// writeMu (or be in a read-only path where concurrent writes are acceptable).
func (s *Store) lookupRunNode(runID, agentID string) (int64, bool) {
	var id int64
	err := s.db.QueryRow(`
		SELECT rn.id FROM run_node rn
		JOIN node n ON n.id = rn.node_id
		WHERE rn.run_id = ? AND n.agent_id = ?
	`, runID, agentID).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}
