package benchmark

// fio JSON parsing.
//
// We only model the subset of fio's JSON we actually consume. fio reports
// completion-latency values in nanoseconds (clat_ns); we convert to microseconds
// for the telemetry sample. Bandwidth is reported in KiB/s.
//
// Two modes share these structs:
//   - Live status: `fio --status-interval=N --output-format=json` prints a
//     sequence of complete JSON documents (one per interval) concatenated
//     together. These carry *cumulative running averages*, which is enough for a
//     live IOPS/latency readout. Use SplitConcatenatedJSON + ParseReport.
//   - Final report: a single JSON document at the end (ideally json+), used for
//     the stored per-run summary.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// FioReport is the top-level fio JSON document.
type FioReport struct {
	FioVersion string   `json:"fio version"`
	Jobs       []FioJob `json:"jobs"`
	// Timestamp is fio's wall-clock time (seconds) when the report was emitted.
	Timestamp int64 `json:"timestamp"`
}

// FioJob is one job's stats within a report. We read read/write sub-objects.
type FioJob struct {
	Jobname string     `json:"jobname"`
	Read    FioRWStats `json:"read"`
	Write   FioRWStats `json:"write"`
}

// FioRWStats holds the read or write statistics we care about.
type FioRWStats struct {
	// IOPS for this direction.
	IOPS float64 `json:"iops"`
	// BW is bandwidth in KiB/s.
	BW float64 `json:"bw"`
	// ClatNs is the completion-latency distribution in nanoseconds.
	ClatNs FioLatNs `json:"clat_ns"`
}

// FioLatNs holds latency mean and percentiles in nanoseconds. The percentile
// map keys are strings like "95.000000".
type FioLatNs struct {
	Mean       float64            `json:"mean"`
	Percentile map[string]float64 `json:"percentile"`
}

// SummaryMetrics is the distilled result for one direction or combined, in the
// units used across the platform (IOPS, KiB/s, microseconds).
type SummaryMetrics struct {
	IOPSRead   float64 `json:"iopsRead"`
	IOPSWrite  float64 `json:"iopsWrite"`
	BWReadKBs  float64 `json:"bwReadKBs"`
	BWWriteKBs float64 `json:"bwWriteKBs"`
	// Latency values are microseconds, taken from the read direction when the
	// workload reads, otherwise from the write direction. For mixed workloads we
	// report read latency as the headline and keep write available separately.
	LatMeanUs float64 `json:"latMeanUs"`
	LatP95Us  float64 `json:"latP95Us"`
	LatP99Us  float64 `json:"latP99Us"`
}

// ParseReport unmarshals a single fio JSON document.
func ParseReport(data []byte) (*FioReport, error) {
	var r FioReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse fio json: %w", err)
	}
	return &r, nil
}

// Summary distills a report's first job into platform units. Returns ok=false if
// the report has no jobs.
func (r *FioReport) Summary() (SummaryMetrics, bool) {
	if len(r.Jobs) == 0 {
		return SummaryMetrics{}, false
	}
	j := r.Jobs[0]
	m := SummaryMetrics{
		IOPSRead:   j.Read.IOPS,
		IOPSWrite:  j.Write.IOPS,
		BWReadKBs:  j.Read.BW,
		BWWriteKBs: j.Write.BW,
	}
	// Choose the latency source: prefer read if there is read activity, else
	// write. clat_ns -> microseconds (divide by 1000).
	lat := j.Read.ClatNs
	if j.Read.IOPS == 0 && j.Write.IOPS > 0 {
		lat = j.Write.ClatNs
	}
	m.LatMeanUs = lat.Mean / 1000.0
	m.LatP95Us = lat.Percentile["95.000000"] / 1000.0
	m.LatP99Us = lat.Percentile["99.000000"] / 1000.0
	return m, true
}

// SplitConcatenatedJSON splits a buffer that may contain several concatenated
// JSON documents (as produced by `fio --status-interval` with JSON output) into
// individual raw documents. It tolerates whitespace between documents and does
// not require a separator. A trailing partial document (not yet complete) is
// returned as the remainder so a streaming caller can prepend it to the next
// read.
func SplitConcatenatedJSON(buf []byte) (docs [][]byte, remainder []byte) {
	dec := json.NewDecoder(bytes.NewReader(buf))
	for {
		start := dec.InputOffset()
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			// No more complete documents; everything from `start` is a partial
			// (or just trailing whitespace). Return it as the remainder.
			rem := bytes.TrimSpace(buf[start:])
			return docs, rem
		}
		// Copy the raw document so it is independent of buf's backing array.
		doc := make([]byte, len(raw))
		copy(doc, raw)
		docs = append(docs, doc)
	}
}
