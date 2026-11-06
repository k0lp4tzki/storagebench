//go:build !linux

package agent

import "github.com/storagebench/storagebench/internal/protocol"

// noopCollector is used on non-Linux platforms (developer workstations). It
// returns an empty sample; the real collector is Linux-only because it reads
// from /proc.
type noopCollector struct{}

func newMetricsCollector() metricsCollector { return noopCollector{} }

func (noopCollector) Collect() protocol.MetricSample { return protocol.MetricSample{} }
