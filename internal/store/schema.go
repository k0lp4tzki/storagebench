package store

// schemaSQL is the full database schema. It is applied idempotently with
// CREATE TABLE IF NOT EXISTS so startup is safe on an existing database. When
// the schema evolves we will add versioned migrations; for the MVP a single
// create script is sufficient.
const schemaSQL = `
-- A node is a host running an agent. Its system info and capabilities are the
-- latest known values; each run additionally freezes a per-run snapshot.
CREATE TABLE IF NOT EXISTS node (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	agent_id      TEXT NOT NULL UNIQUE,
	hostname      TEXT NOT NULL DEFAULT '',
	system_info   TEXT NOT NULL DEFAULT '{}',   -- JSON protocol.SystemInfo
	capabilities  TEXT NOT NULL DEFAULT '{}',   -- JSON protocol.Capabilities
	last_seen     TEXT NOT NULL DEFAULT ''
);

-- A reusable test profile (resolved parameters stored as JSON).
CREATE TABLE IF NOT EXISTS test_profile (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT NOT NULL,
	params      TEXT NOT NULL,                  -- JSON benchmark spec
	created_at  TEXT NOT NULL
);

-- A test run is one execution, possibly across several nodes.
CREATE TABLE IF NOT EXISTS test_run (
	id               TEXT PRIMARY KEY,          -- run_id (UUID-like)
	profile_id       INTEGER,
	name             TEXT NOT NULL DEFAULT '',
	status           TEXT NOT NULL DEFAULT 'created',
	resolved_config  TEXT NOT NULL DEFAULT '{}',-- JSON: exact config used
	started_at       TEXT NOT NULL DEFAULT '',
	ended_at         TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (profile_id) REFERENCES test_profile(id) ON DELETE SET NULL
);

-- Per-node participation in a run. Freezes the node's system snapshot so results
-- remain reproducible even if the node changes later.
CREATE TABLE IF NOT EXISTS run_node (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id           TEXT NOT NULL,
	node_id          INTEGER NOT NULL,
	phase            TEXT NOT NULL DEFAULT 'assigned',
	phase_message    TEXT NOT NULL DEFAULT '',
	system_snapshot  TEXT NOT NULL DEFAULT '{}',-- JSON at assignment time
	fio_summary      TEXT NOT NULL DEFAULT '',  -- JSON final fio result
	UNIQUE(run_id, node_id),
	FOREIGN KEY (run_id) REFERENCES test_run(id) ON DELETE CASCADE,
	FOREIGN KEY (node_id) REFERENCES node(id) ON DELETE CASCADE
);

-- Time-series metric samples. Indexed by run_node for fast per-node queries.
CREATE TABLE IF NOT EXISTS metric_sample (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	run_node_id  INTEGER NOT NULL,
	ts_wall      INTEGER NOT NULL,              -- unix millis, UTC
	ts_mono      INTEGER NOT NULL DEFAULT 0,    -- agent monotonic nanos
	iops_r       REAL NOT NULL DEFAULT 0,
	iops_w       REAL NOT NULL DEFAULT 0,
	bw_r         REAL NOT NULL DEFAULT 0,
	bw_w         REAL NOT NULL DEFAULT 0,
	lat_mean     REAL NOT NULL DEFAULT 0,
	lat_p95      REAL NOT NULL DEFAULT 0,
	lat_p99      REAL NOT NULL DEFAULT 0,
	qdepth       REAL NOT NULL DEFAULT 0,
	cpu          REAL NOT NULL DEFAULT 0,
	mem          REAL NOT NULL DEFAULT 0,
	load1        REAL NOT NULL DEFAULT 0,
	disk_util    REAL NOT NULL DEFAULT 0,
	net_rx       REAL NOT NULL DEFAULT 0,
	net_tx       REAL NOT NULL DEFAULT 0,
	FOREIGN KEY (run_node_id) REFERENCES run_node(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_metric_run_node_ts ON metric_sample(run_node_id, ts_wall);

-- Discrete events (errors, kernel messages, fault markers).
CREATE TABLE IF NOT EXISTS event (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	run_node_id  INTEGER,
	agent_id     TEXT NOT NULL DEFAULT '',
	ts_wall      INTEGER NOT NULL,
	ts_mono      INTEGER NOT NULL DEFAULT 0,
	severity     TEXT NOT NULL DEFAULT 'info',
	source       TEXT NOT NULL DEFAULT '',
	message      TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (run_node_id) REFERENCES run_node(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_event_run_node_ts ON event(run_node_id, ts_wall);

-- Fault-injection actions, correlated with the run timeline. Populated in the
-- resilience-testing phase; table exists now so the schema is stable.
CREATE TABLE IF NOT EXISTS fault_action (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id       TEXT NOT NULL,
	node_id      INTEGER,
	type         TEXT NOT NULL,
	target       TEXT NOT NULL DEFAULT '',
	ts_inject    INTEGER NOT NULL DEFAULT 0,
	ts_revert    INTEGER NOT NULL DEFAULT 0,
	detail       TEXT NOT NULL DEFAULT '{}',
	FOREIGN KEY (run_id) REFERENCES test_run(id) ON DELETE CASCADE
);
`
