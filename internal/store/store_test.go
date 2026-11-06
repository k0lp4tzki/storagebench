package store

import (
	"path/filepath"
	"testing"

	"github.com/storagebench/storagebench/internal/protocol"
)

// openTemp opens a store backed by a temp-dir database file.
func openTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestUpsertNodeAndList(t *testing.T) {
	st := openTemp(t)

	hello := protocol.Hello{
		AgentID: "node-a",
		System: protocol.SystemInfo{
			Hostname: "node-a", OS: "linux", Distro: "SLES 15 SP5", Arch: "amd64",
		},
		Capabilities: protocol.Capabilities{Fio: true, FioVersion: "3.36"},
	}
	if err := st.UpsertNode("node-a", hello); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	// Upsert again with changed info to confirm ON CONFLICT update works.
	hello.System.KernelVers = "5.14.21"
	if err := st.UpsertNode("node-a", hello); err != nil {
		t.Fatalf("UpsertNode (update): %v", err)
	}

	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Capabilities.FioVersion != "3.36" {
		t.Errorf("fio version = %q, want 3.36", nodes[0].Capabilities.FioVersion)
	}
	if nodes[0].System.KernelVers != "5.14.21" {
		t.Errorf("kernel = %q, want updated value", nodes[0].System.KernelVers)
	}
}

// TestIngestMetricsRequiresRunNode confirms that ingesting metrics for an
// unknown (run, agent) pair is a safe no-op rather than an error/panic.
func TestIngestMetricsUnknownRunIsNoop(t *testing.T) {
	st := openTemp(t)
	// No node, no run exists yet.
	st.IngestMetrics("ghost", protocol.MetricBatch{
		RunID:   "missing",
		Seq:     1,
		Samples: []protocol.MetricSample{{TSWallMilli: 1}},
	})
	// Nothing to assert beyond "did not panic / error"; the call must be safe.
}

func TestListRunsEmpty(t *testing.T) {
	st := openTemp(t)
	runs, err := st.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected no runs, got %d", len(runs))
	}
}
