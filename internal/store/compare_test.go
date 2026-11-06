package store

import (
	"testing"

	"github.com/storagebench/storagebench/internal/protocol"
)

// TestGetRunComparison verifies that the comparison aggregates per-host fio
// summaries into run-level totals (throughput summed, tail latency worst-case,
// mean latency averaged) and preserves the requested run order.
func TestGetRunComparison(t *testing.T) {
	st := openTemp(t)

	// Two nodes.
	for _, id := range []string{"node-a", "node-b"} {
		if err := st.UpsertNode(id, protocol.Hello{
			AgentID: id,
			System:  protocol.SystemInfo{Hostname: id},
		}); err != nil {
			t.Fatalf("UpsertNode %s: %v", id, err)
		}
	}

	// Run 1: two hosts.
	if err := st.CreateRun("run-1", "Baseline", nil); err != nil {
		t.Fatalf("CreateRun run-1: %v", err)
	}
	for _, id := range []string{"node-a", "node-b"} {
		if err := st.AddRunNode("run-1", id); err != nil {
			t.Fatalf("AddRunNode run-1/%s: %v", id, err)
		}
	}
	st.OnTestResult("node-a", protocol.TestResult{
		RunID: "run-1", IOPSRead: 10000, BWReadKBs: 40000,
		LatMeanUs: 100, LatP99Us: 500, LatP999Us: 1500,
	})
	st.OnTestResult("node-b", protocol.TestResult{
		RunID: "run-1", IOPSRead: 20000, BWReadKBs: 80000,
		LatMeanUs: 200, LatP99Us: 800, LatP999Us: 2500,
	})

	// Run 2: one host.
	if err := st.CreateRun("run-2", "After change", nil); err != nil {
		t.Fatalf("CreateRun run-2: %v", err)
	}
	if err := st.AddRunNode("run-2", "node-a"); err != nil {
		t.Fatalf("AddRunNode run-2: %v", err)
	}
	st.OnTestResult("node-a", protocol.TestResult{
		RunID: "run-2", IOPSRead: 30000, BWReadKBs: 120000,
		LatMeanUs: 90, LatP99Us: 400, LatP999Us: 1000,
	})

	cmp, err := st.GetRunComparison([]string{"run-2", "run-1"})
	if err != nil {
		t.Fatalf("GetRunComparison: %v", err)
	}
	if len(cmp) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(cmp))
	}

	// Order preserved: run-2 first.
	if cmp[0].ID != "run-2" || cmp[1].ID != "run-1" {
		t.Fatalf("run order not preserved: %s, %s", cmp[0].ID, cmp[1].ID)
	}

	// run-1 aggregate: IOPS summed, worst-case tail latency, averaged mean.
	r1 := cmp[1]
	if got := r1.Aggregate.IOPSRead; got != 30000 {
		t.Errorf("run-1 aggregate iops_read = %v, want 30000", got)
	}
	if got := r1.Aggregate.BWReadKBs; got != 120000 {
		t.Errorf("run-1 aggregate bw_read = %v, want 120000", got)
	}
	if got := r1.Aggregate.LatP999Us; got != 2500 {
		t.Errorf("run-1 aggregate lat_p999 = %v, want 2500 (worst case)", got)
	}
	if got := r1.Aggregate.LatMeanUs; got != 150 {
		t.Errorf("run-1 aggregate lat_mean = %v, want 150 (avg of 100,200)", got)
	}
	if len(r1.Nodes) != 2 {
		t.Errorf("run-1 should have 2 nodes, got %d", len(r1.Nodes))
	}

	// Unknown ids are skipped.
	cmp2, err := st.GetRunComparison([]string{"nope", "run-2"})
	if err != nil {
		t.Fatalf("GetRunComparison (with unknown): %v", err)
	}
	if len(cmp2) != 1 || cmp2[0].ID != "run-2" {
		t.Fatalf("expected only run-2, got %d runs", len(cmp2))
	}
}
