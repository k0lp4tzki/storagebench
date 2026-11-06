package store

import (
	"encoding/json"
	"math"

	"github.com/storagebench/storagebench/internal/protocol"
)

// FioSummaryValues mirrors the fio_summary JSON stored per run_node. It is the
// aggregate result of one node's benchmark and the unit of cross-run comparison.
type FioSummaryValues struct {
	ExitCode   int     `json:"exitCode"`
	IOPSRead   float64 `json:"iopsRead"`
	IOPSWrite  float64 `json:"iopsWrite"`
	BWReadKBs  float64 `json:"bwReadKBs"`
	BWWriteKBs float64 `json:"bwWriteKBs"`
	LatMeanUs  float64 `json:"latMeanUs"`
	LatP95Us   float64 `json:"latP95Us"`
	LatP99Us   float64 `json:"latP99Us"`
	LatP999Us  float64 `json:"latP999Us"`
}

// CompareNode is one node's contribution to a run in a comparison, including a
// derived stability metric (coefficient of variation of total IOPS over time).
type CompareNode struct {
	AgentID  string           `json:"agentId"`
	Hostname string           `json:"hostname"`
	Phase    string           `json:"phase"`
	Summary  FioSummaryValues `json:"summary"`
	// IOPSCv is stddev/mean of (read+write) IOPS over the run; -1 if unknown.
	IOPSCv float64 `json:"iopsCv"`
}

// CompareRun is one run in a comparison: its identity plus per-node results and
// a run-level aggregate (sum of IOPS/bandwidth, worst-case tail latency) so runs
// with differing host counts can still be compared at a glance.
type CompareRun struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Status    string           `json:"status"`
	StartedAt string           `json:"startedAt"`
	EndedAt   string           `json:"endedAt"`
	Nodes     []CompareNode    `json:"nodes"`
	Aggregate FioSummaryValues `json:"aggregate"`
}

// GetRunComparison returns a comparison view for the given run ids, preserving
// the requested order. Unknown ids are skipped.
func (s *Store) GetRunComparison(runIDs []string) ([]CompareRun, error) {
	out := make([]CompareRun, 0, len(runIDs))
	for _, id := range runIDs {
		d, ok, err := s.GetRun(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		metrics, _ := s.GetRunMetrics(id) // best-effort; nil is fine

		cr := CompareRun{
			ID:        d.ID,
			Name:      d.Name,
			Status:    d.Status,
			StartedAt: d.StartedAt,
			EndedAt:   d.EndedAt,
		}
		for _, nd := range d.Nodes {
			var sum FioSummaryValues
			if len(nd.FioSummary) > 0 {
				_ = json.Unmarshal(nd.FioSummary, &sum)
			}
			cn := CompareNode{
				AgentID:  nd.AgentID,
				Hostname: nd.Hostname,
				Phase:    nd.Phase,
				Summary:  sum,
				IOPSCv:   iopsCV(metrics[nd.AgentID]),
			}
			cr.Nodes = append(cr.Nodes, cn)

			// Run-level aggregate: sum throughput across hosts, take the worst
			// (highest) tail latencies, and average the mean latency.
			cr.Aggregate.IOPSRead += sum.IOPSRead
			cr.Aggregate.IOPSWrite += sum.IOPSWrite
			cr.Aggregate.BWReadKBs += sum.BWReadKBs
			cr.Aggregate.BWWriteKBs += sum.BWWriteKBs
			cr.Aggregate.LatP95Us = math.Max(cr.Aggregate.LatP95Us, sum.LatP95Us)
			cr.Aggregate.LatP99Us = math.Max(cr.Aggregate.LatP99Us, sum.LatP99Us)
			cr.Aggregate.LatP999Us = math.Max(cr.Aggregate.LatP999Us, sum.LatP999Us)
		}
		if n := len(cr.Nodes); n > 0 {
			var meanSum float64
			for _, cn := range cr.Nodes {
				meanSum += cn.Summary.LatMeanUs
			}
			cr.Aggregate.LatMeanUs = meanSum / float64(n)
		}
		out = append(out, cr)
	}
	return out, nil
}

// iopsCV returns the coefficient of variation (stddev/mean) of total IOPS over
// the samples, a unit-free stability measure. Returns -1 when undefined.
func iopsCV(samples []protocol.MetricSample) float64 {
	if len(samples) < 3 {
		return -1
	}
	xs := make([]float64, 0, len(samples))
	for _, m := range samples {
		xs = append(xs, m.IOPSRead+m.IOPSWrite)
	}
	var mean float64
	for _, v := range xs {
		mean += v
	}
	mean /= float64(len(xs))
	if mean <= 0 {
		return -1
	}
	var variance float64
	for _, v := range xs {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(xs))
	return math.Sqrt(variance) / mean
}
