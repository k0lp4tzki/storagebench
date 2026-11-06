// Package benchmark defines the abstracted, validated description of a storage
// test and turns it into a safe fio jobfile. Users of the platform configure
// tests through these high-level fields; they never see or type fio parameters
// or shell commands.
//
// Validation is strict and allowlist-based. Every enum-like field is checked
// against a fixed set of accepted values, and every target path is validated,
// so a malformed or hostile spec cannot produce a dangerous fio invocation.
// This is the first line of defense that complements the privilege-broker.
package benchmark

import (
	"fmt"
	"strings"
)

// Workload is the I/O pattern to generate. These map directly to fio's rw modes.
type Workload string

const (
	WorkloadRandRead   Workload = "randread"
	WorkloadRandWrite  Workload = "randwrite"
	WorkloadRandRW     Workload = "randrw"
	WorkloadSeqRead    Workload = "read"
	WorkloadSeqWrite   Workload = "write"
	WorkloadSeqRW      Workload = "rw"
)

// validWorkloads is the allowlist of accepted workloads.
var validWorkloads = map[Workload]bool{
	WorkloadRandRead: true, WorkloadRandWrite: true, WorkloadRandRW: true,
	WorkloadSeqRead: true, WorkloadSeqWrite: true, WorkloadSeqRW: true,
}

// isMixed reports whether the workload mixes reads and writes (so rwmixread
// applies).
func (w Workload) isMixed() bool {
	return w == WorkloadRandRW || w == WorkloadSeqRW
}

// isWrite reports whether the workload performs any writes. Used by safety
// checks: a write workload against a device requires explicit opt-in.
func (w Workload) isWrite() bool {
	switch w {
	case WorkloadRandWrite, WorkloadSeqWrite, WorkloadRandRW, WorkloadSeqRW:
		return true
	default:
		return false
	}
}

// IOEngine is the fio I/O engine. We allowlist the engines we support and that
// are meaningful on Linux block/file storage.
type IOEngine string

const (
	EngineLibaio  IOEngine = "libaio"
	EngineIoUring IOEngine = "io_uring"
	EnginePsync   IOEngine = "psync"
	EngineSync    IOEngine = "sync"
)

var validEngines = map[IOEngine]bool{
	EngineLibaio: true, EngineIoUring: true, EnginePsync: true, EngineSync: true,
}

// TargetKind distinguishes a file target (a path inside a mounted filesystem)
// from a raw block device. The distinction matters for safety: raw devices are
// far more dangerous and get stricter checks.
type TargetKind string

const (
	// TargetFile is a regular file inside a filesystem (fio uses/creates it with
	// a bounded size). This is the default and the safest option.
	TargetFile TargetKind = "file"
	// TargetDevice is a raw block device (e.g. /dev/sdX, /dev/mapper/...). Using
	// it requires an explicit allowlist entry on the agent and, for writes, an
	// explicit destructive opt-in.
	TargetDevice TargetKind = "device"
)

var validTargetKinds = map[TargetKind]bool{TargetFile: true, TargetDevice: true}

// Spec is the full, abstracted test specification. It is serialized to JSON and
// carried with every run (frozen as resolved_config) so results are
// reproducible.
type Spec struct {
	// Name is a human-readable label for the test.
	Name string `json:"name"`

	// --- Target ---

	// TargetKind is "file" or "device".
	TargetKind TargetKind `json:"targetKind"`
	// TargetPath is the file path or device node fio will operate on. For files,
	// fio will create the file if needed (sized by FileSizeMiB).
	TargetPath string `json:"targetPath"`
	// FileSizeMiB is the size of the test file (file targets only). Ignored for
	// device targets, where the whole device (or SizeMiB) is used.
	FileSizeMiB int64 `json:"fileSizeMiB,omitempty"`
	// SizeMiB optionally limits the region tested on a device target. 0 means
	// "use the device's full size".
	SizeMiB int64 `json:"sizeMiB,omitempty"`

	// --- I/O configuration ---

	// Workload is the I/O pattern.
	Workload Workload `json:"workload"`
	// BlockSize is the fio block size, e.g. "4k", "64k", "1m". Validated against
	// an allowlist of common sizes.
	BlockSize string `json:"blockSize"`
	// RWMixRead is the read percentage for mixed workloads (0..100). Only used
	// when Workload is mixed.
	RWMixRead int `json:"rwMixRead,omitempty"`
	// IODepth is the queue depth per job (iodepth). 1 for latency-style tests.
	IODepth int `json:"ioDepth"`
	// NumJobs is the number of parallel fio jobs (numjobs).
	NumJobs int `json:"numJobs"`
	// Engine is the fio ioengine.
	Engine IOEngine `json:"engine"`
	// DirectIO selects O_DIRECT (direct=1) vs. buffered I/O (direct=0).
	DirectIO bool `json:"directIO"`

	// --- Load configuration ---

	// RuntimeSeconds is the measured test duration (runtime, time_based).
	RuntimeSeconds int `json:"runtimeSeconds"`
	// RampSeconds is a warm-up period excluded from results (ramp_time).
	RampSeconds int `json:"rampSeconds,omitempty"`
	// TargetIOPS optionally caps the rate (rate_iops). 0 means unthrottled.
	TargetIOPS int `json:"targetIOPS,omitempty"`
	// MaxBandwidthKBs optionally caps bandwidth in KiB/s (rate). 0 means
	// unthrottled.
	MaxBandwidthKBs int `json:"maxBandwidthKBs,omitempty"`

	// --- Safety ---

	// AllowDestructive must be true to run a write workload against a device
	// target. It is a deliberate, explicit opt-in; without it such a spec is
	// rejected at validation time.
	AllowDestructive bool `json:"allowDestructive,omitempty"`
}

// validBlockSizes is the allowlist of accepted block sizes. Keeping this fixed
// avoids free-form strings reaching the jobfile.
var validBlockSizes = map[string]bool{
	"512": true, "1k": true, "4k": true, "8k": true, "16k": true,
	"32k": true, "64k": true, "128k": true, "256k": true, "512k": true,
	"1m": true, "2m": true, "4m": true,
}

// Limits used by validation. These are conservative upper bounds to prevent
// absurd or resource-exhausting configurations.
const (
	maxIODepth        = 1024
	maxNumJobs        = 256
	maxRuntimeSeconds = 24 * 60 * 60 // 24h
	maxRampSeconds    = 60 * 60      // 1h
)

// Validate checks the spec against the allowlists and bounds. It returns a
// descriptive error on the first problem found. A spec that passes Validate is
// safe to turn into a jobfile.
func (s *Spec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if !validTargetKinds[s.TargetKind] {
		return fmt.Errorf("invalid targetKind %q", s.TargetKind)
	}
	if err := validateTargetPath(s.TargetKind, s.TargetPath); err != nil {
		return err
	}
	if !validWorkloads[s.Workload] {
		return fmt.Errorf("invalid workload %q", s.Workload)
	}
	if !validBlockSizes[strings.ToLower(s.BlockSize)] {
		return fmt.Errorf("invalid blockSize %q", s.BlockSize)
	}
	if !validEngines[s.Engine] {
		return fmt.Errorf("invalid engine %q", s.Engine)
	}
	if s.Workload.isMixed() {
		if s.RWMixRead < 0 || s.RWMixRead > 100 {
			return fmt.Errorf("rwMixRead must be between 0 and 100, got %d", s.RWMixRead)
		}
	}
	if s.IODepth < 1 || s.IODepth > maxIODepth {
		return fmt.Errorf("ioDepth must be between 1 and %d, got %d", maxIODepth, s.IODepth)
	}
	if s.NumJobs < 1 || s.NumJobs > maxNumJobs {
		return fmt.Errorf("numJobs must be between 1 and %d, got %d", maxNumJobs, s.NumJobs)
	}
	if s.RuntimeSeconds < 1 || s.RuntimeSeconds > maxRuntimeSeconds {
		return fmt.Errorf("runtimeSeconds must be between 1 and %d, got %d", maxRuntimeSeconds, s.RuntimeSeconds)
	}
	if s.RampSeconds < 0 || s.RampSeconds > maxRampSeconds {
		return fmt.Errorf("rampSeconds must be between 0 and %d, got %d", maxRampSeconds, s.RampSeconds)
	}
	if s.TargetIOPS < 0 {
		return fmt.Errorf("targetIOPS must not be negative")
	}
	if s.MaxBandwidthKBs < 0 {
		return fmt.Errorf("maxBandwidthKBs must not be negative")
	}
	switch s.TargetKind {
	case TargetFile:
		if s.FileSizeMiB < 1 {
			return fmt.Errorf("fileSizeMiB must be at least 1 for file targets")
		}
	case TargetDevice:
		if s.SizeMiB < 0 {
			return fmt.Errorf("sizeMiB must not be negative")
		}
		// Writing to a device is destructive and must be explicitly authorized.
		if s.Workload.isWrite() && !s.AllowDestructive {
			return fmt.Errorf("write workload on a device target requires allowDestructive=true")
		}
	}
	return nil
}

// validateTargetPath performs structural validation of the target path. It does
// not touch the filesystem (that, plus allowlist enforcement against actual
// devices/mountpoints, happens on the agent which knows the host). It rejects
// obviously unsafe shapes early.
func validateTargetPath(kind TargetKind, path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("targetPath must not be empty")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("targetPath must be absolute, got %q", p)
	}
	// Reject NUL and newline which have no place in a path and could corrupt a
	// jobfile. Other characters are allowed; the jobfile writer quotes values.
	if strings.ContainsAny(p, "\x00\n\r") {
		return fmt.Errorf("targetPath contains invalid characters")
	}
	// Disallow path traversal segments to avoid surprising escapes.
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("targetPath must not contain '..' segments")
		}
	}
	if kind == TargetDevice && !strings.HasPrefix(p, "/dev/") {
		return fmt.Errorf("device targetPath must be under /dev/, got %q", p)
	}
	return nil
}

// NormalizedBlockSize returns the lower-cased block size (fio is case-sensitive
// for suffixes but accepts lowercase; we normalize for consistency).
func (s *Spec) NormalizedBlockSize() string {
	return strings.ToLower(s.BlockSize)
}
