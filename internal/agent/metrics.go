package agent

import "github.com/storagebench/storagebench/internal/protocol"

// metricsCollector produces a host metric sample. The benchmark-derived fields
// (IOPS, bandwidth, latency, queue depth) are filled by the fio runner in a
// later phase; this collector supplies the host-level fields (CPU, memory, load,
// disk/network throughput).
type metricsCollector interface {
	// Collect returns a sample with host metrics populated. Timestamps are set
	// by the caller (the Sampler).
	Collect() protocol.MetricSample
}
