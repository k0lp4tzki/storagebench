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

-- A reusable test profile: a named, ordered collection of test steps. Each step
-- is a benchmark spec (without a bound target host); steps are stored as a JSON
-- array so the profile is a single, versionable document.
CREATE TABLE IF NOT EXISTS test_profile (
	id          TEXT PRIMARY KEY,               -- profile id
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	steps       TEXT NOT NULL DEFAULT '[]',     -- JSON array of ProfileStep
	created_at  TEXT NOT NULL DEFAULT '',
	updated_at  TEXT NOT NULL DEFAULT ''
);

-- A batch is one execution of a profile against a set of hosts. It groups the
-- individual runs (one run per profile step) so results can be viewed together.
CREATE TABLE IF NOT EXISTS test_batch (
	id           TEXT PRIMARY KEY,              -- batch id
	profile_id   TEXT,
	name         TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'created',-- created|running|completed|failed|aborted
	agent_ids    TEXT NOT NULL DEFAULT '[]',    -- JSON array of target agent ids
	total_steps  INTEGER NOT NULL DEFAULT 0,
	step_index   INTEGER NOT NULL DEFAULT 0,    -- 0-based index of the active step
	started_at   TEXT NOT NULL DEFAULT '',
	ended_at     TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (profile_id) REFERENCES test_profile(id) ON DELETE SET NULL
);

-- A test run is one execution, possibly across several nodes. A run may belong to
-- a batch (one run per profile step) via batch_id.
CREATE TABLE IF NOT EXISTS test_run (
	id               TEXT PRIMARY KEY,          -- run_id (UUID-like)
	profile_id       TEXT,
	batch_id         TEXT,
	batch_step       INTEGER NOT NULL DEFAULT 0,-- 0-based step index within the batch
	name             TEXT NOT NULL DEFAULT '',
	status           TEXT NOT NULL DEFAULT 'created',
	resolved_config  TEXT NOT NULL DEFAULT '{}',-- JSON: exact config used
	started_at       TEXT NOT NULL DEFAULT '',
	ended_at         TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (profile_id) REFERENCES test_profile(id) ON DELETE SET NULL,
	FOREIGN KEY (batch_id) REFERENCES test_batch(id) ON DELETE SET NULL
);
-- NOTE: the idx_run_batch index is created in migrate() *after* the additive
-- ALTER TABLE statements, so that existing databases (whose test_run predates
-- batch_id) get the column added before the index references it.

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

-- Fault-injection actions, correlated with the run timeline (resilience phase).
-- A row is created on inject (ts_revert=0) and stamped with ts_revert when the
-- fault is removed, so the UI can draw inject/revert markers on the charts.
CREATE TABLE IF NOT EXISTS fault_action (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id       TEXT NOT NULL,
	fault_id     TEXT NOT NULL DEFAULT '',
	agent_id     TEXT NOT NULL DEFAULT '',
	kind         TEXT NOT NULL DEFAULT '',
	iface        TEXT NOT NULL DEFAULT '',
	detail       TEXT NOT NULL DEFAULT '',
	ts_inject    INTEGER NOT NULL DEFAULT 0,
	ts_revert    INTEGER NOT NULL DEFAULT 0,
	ttl_seconds  INTEGER NOT NULL DEFAULT 0,    -- watchdog TTL; 0 = none/unknown
	FOREIGN KEY (run_id) REFERENCES test_run(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fault_run ON fault_action(run_id, ts_inject);
`
