package benchmark

import (
	"fmt"
	"path/filepath"
	"strings"
)

// JobName is the fixed fio job section name we use. Having a stable, known name
// makes parsing the JSON output deterministic (we read jobs[0]).
const JobName = "storagebench"

// RenderedJob bundles a generated fio jobfile with the companion paths fio will
// write to. The runner creates the jobfile on disk and passes workDir so fio's
// log files land in a known place for parsing.
type RenderedJob struct {
	// JobfileContent is the INI-format fio jobfile.
	JobfileContent string
	// LogPrefix is the fio write_*_log prefix (without the ".log" suffix and
	// without fio's job/index suffixes). fio appends e.g. "_bw.1.log".
	LogPrefix string
}

// BuildJobfile turns a validated Spec into a fio jobfile plus the companion log
// prefix. workDir must be an absolute directory the agent controls; fio will
// write its per-interval logs there.
//
// The caller must have validated the spec (BuildJobfile re-validates to be safe
// and never emits a jobfile for an invalid spec).
func BuildJobfile(s *Spec, workDir string) (RenderedJob, error) {
	if err := s.Validate(); err != nil {
		return RenderedJob{}, fmt.Errorf("invalid spec: %w", err)
	}
	if !filepath.IsAbs(workDir) {
		return RenderedJob{}, fmt.Errorf("workDir must be absolute, got %q", workDir)
	}

	logPrefix := filepath.Join(workDir, "fio")

	var b strings.Builder

	// Global section: options shared by all jobs. Using a [global] section keeps
	// the job section minimal and is the idiomatic fio layout.
	b.WriteString("[global]\n")
	writeKV(&b, "name", JobName)
	writeKV(&b, "ioengine", string(s.Engine))
	writeKV(&b, "rw", string(s.Workload))
	writeKV(&b, "bs", s.NormalizedBlockSize())
	writeKVInt(&b, "iodepth", s.IODepth)
	writeKVInt(&b, "numjobs", s.NumJobs)
	writeKVInt(&b, "direct", boolToInt(s.DirectIO))

	// time_based + runtime makes fio run for a fixed wall-clock duration,
	// looping over the data region as needed.
	writeKV(&b, "time_based", "1")
	writeKVInt(&b, "runtime", s.RuntimeSeconds)
	if s.RampSeconds > 0 {
		writeKVInt(&b, "ramp_time", s.RampSeconds)
	}

	if s.Workload.isMixed() {
		writeKVInt(&b, "rwmixread", s.RWMixRead)
	}

	// Rate limiting (optional).
	if s.TargetIOPS > 0 {
		writeKVInt(&b, "rate_iops", s.TargetIOPS)
	}
	if s.MaxBandwidthKBs > 0 {
		// fio's "rate" is in bytes/s unless suffixed; use "k" for KiB/s.
		writeKV(&b, "rate", fmt.Sprintf("%dk", s.MaxBandwidthKBs))
	}

	// Latency percentiles: enable completion-latency percentiles and set the
	// list to the ones we report (P95, P99), so the final JSON contains them.
	writeKV(&b, "clat_percentiles", "1")
	writeKV(&b, "percentile_list", "95.000000:99.000000:99.900000")

	// Per-interval logs for true time-series curves (status-interval only gives
	// running averages). log_avg_msec aggregates each log entry over a window.
	writeKVInt(&b, "log_avg_msec", 1000)
	writeKV(&b, "write_bw_log", logPrefix)
	writeKV(&b, "write_iops_log", logPrefix)
	writeKV(&b, "write_lat_log", logPrefix)

	// Group reporting so numjobs>1 is summarized as one result.
	writeKV(&b, "group_reporting", "1")

	// Note: fio's read-only guard is NOT a jobfile keyword; it is the CLI flag
	// --readonly. The runner adds it for read-only workloads (see Runner.Run).

	// Target: filename for both file and device targets. For file targets we set
	// size so fio creates/uses a bounded file; for device targets we optionally
	// limit the region with size.
	b.WriteString("\n[" + JobName + "]\n")
	writeKV(&b, "filename", s.TargetPath)
	switch s.TargetKind {
	case TargetFile:
		writeKV(&b, "size", fmt.Sprintf("%dM", s.FileSizeMiB))
		// Allow fio to create its own test file if it does not exist yet. This is
		// required for read-only workloads against a fresh file target.
		writeKV(&b, "allow_file_create", "1")
		// Materialize the whole file up front with a fast native fallocate so the
		// file-layout phase does not steal from the measured, time-based runtime
		// (otherwise a fresh read test spends its time creating the file and
		// reports ~0 IOPS). The runner additionally pre-creates the file.
		writeKV(&b, "fallocate", "native")
	case TargetDevice:
		if s.SizeMiB > 0 {
			writeKV(&b, "size", fmt.Sprintf("%dM", s.SizeMiB))
		}
	}

	return RenderedJob{
		JobfileContent: b.String(),
		LogPrefix:      logPrefix,
	}, nil
}

// writeKV writes a "key=value" line. Values come from validated/allowlisted
// fields or from numeric formatting, so no shell/INI escaping of untrusted free
// text is required; the only free-form value (path) is validated for newlines in
// the spec, which would otherwise be the sole way to inject extra INI lines.
func writeKV(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(value)
	b.WriteByte('\n')
}

func writeKVInt(b *strings.Builder, key string, value int) {
	writeKV(b, key, fmt.Sprintf("%d", value))
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
