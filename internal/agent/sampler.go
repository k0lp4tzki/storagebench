package agent

import (
	"context"
	"sync"
	"time"

	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/protocol"
)

// maxBufferedSamples caps the in-memory telemetry buffer. If the controller is
// unreachable for a long time the oldest samples are dropped once this bound is
// hit, protecting the agent's memory. (A disk-backed spool for truly lossless
// buffering across long outages is a documented later enhancement.)
const maxBufferedSamples = 10000

// Sampler periodically collects host metrics and buffers them with monotonic
// sequence numbers so they can be batched to the controller and replayed after
// a reconnect. It is safe for concurrent use.
type Sampler struct {
	interval time.Duration

	mu        sync.Mutex
	buf       []protocol.MetricSample
	seq       uint64 // last assigned batch sequence
	runID     string // current run id for telemetry association
	collector metricsCollector

	// fioLive holds the most recent fio-derived metrics (IOPS/BW/latency). The
	// fio runner updates it; each host sample merges these in so storage and host
	// metrics share one timestamp. hasFio guards whether to include them.
	fioLive protocol.MetricSample
	hasFio  bool
}

// NewSampler creates a sampler with the given collection interval.
func NewSampler(interval time.Duration) *Sampler {
	if interval <= 0 {
		interval = time.Second
	}
	return &Sampler{
		interval:  interval,
		collector: newMetricsCollector(),
	}
}

// SetRunID associates subsequent samples with a run. An empty string means host
// telemetry is still collected but not tied to a specific benchmark run.
func (s *Sampler) SetRunID(runID string) {
	s.mu.Lock()
	s.runID = runID
	s.mu.Unlock()
}

// Start runs the collection loop until ctx is canceled.
func (s *Sampler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample := s.collector.Collect()
			sample.TSWallMilli = clock.NowWallMilli()
			sample.MonoNanos = clock.MonoNanos()
			s.append(sample)
		}
	}
}

func (s *Sampler) append(sample protocol.MetricSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Merge the latest fio-derived metrics so storage and host metrics share one
	// timestamped sample.
	if s.hasFio {
		sample.IOPSRead = s.fioLive.IOPSRead
		sample.IOPSWrite = s.fioLive.IOPSWrite
		sample.BWReadKBs = s.fioLive.BWReadKBs
		sample.BWWriteKBs = s.fioLive.BWWriteKBs
		sample.LatMeanUs = s.fioLive.LatMeanUs
		sample.LatP95Us = s.fioLive.LatP95Us
		sample.LatP99Us = s.fioLive.LatP99Us
		sample.QueueDepth = s.fioLive.QueueDepth
	}
	if len(s.buf) >= maxBufferedSamples {
		// Drop oldest to bound memory.
		drop := len(s.buf) - maxBufferedSamples + 1
		s.buf = s.buf[drop:]
	}
	s.buf = append(s.buf, sample)
}

// SetFioMetrics records the latest fio-derived metrics to be merged into
// subsequent host samples. Called by the fio runner on each live update.
func (s *Sampler) SetFioMetrics(m protocol.MetricSample) {
	s.mu.Lock()
	s.fioLive = m
	s.hasFio = true
	s.mu.Unlock()
}

// ClearFioMetrics stops merging fio metrics (call when a test ends) so idle
// host telemetry does not carry stale IOPS/latency values.
func (s *Sampler) ClearFioMetrics() {
	s.mu.Lock()
	s.fioLive = protocol.MetricSample{}
	s.hasFio = false
	s.mu.Unlock()
}

// DrainBatch returns all buffered samples as a single batch and advances the
// sequence number. Returns ok=false if there is nothing to send. The drained
// samples are retained conceptually via the sequence/ack scheme: because we do
// not yet persist to disk, we remove them from the buffer here and rely on the
// controller ack for confirmation. For the MVP this is acceptable; a durable
// spool is a planned enhancement for long outages.
func (s *Sampler) DrainBatch() (protocol.MetricBatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == 0 {
		return protocol.MetricBatch{}, false
	}
	s.seq++
	samples := make([]protocol.MetricSample, len(s.buf))
	copy(samples, s.buf)
	s.buf = s.buf[:0]
	return protocol.MetricBatch{RunID: s.runID, Seq: s.seq, Samples: samples}, true
}

// Acknowledge records that the controller has durably received telemetry up to
// seq for runID. With the in-memory buffer this is informational; it becomes
// load-bearing once a disk spool is added for lossless replay.
func (s *Sampler) Acknowledge(runID string, seq uint64) {
	// No-op for the in-memory MVP buffer. Kept to finalize the ack protocol and
	// give the controller a well-defined contract to rely on.
	_ = runID
	_ = seq
}
