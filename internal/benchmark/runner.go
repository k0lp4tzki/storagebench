package benchmark

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// LiveUpdate is delivered for each parsed live status document while fio runs.
// The values are cumulative running averages (fio's status-interval semantics).
type LiveUpdate struct {
	Metrics SummaryMetrics
	At      time.Time
}

// Result is the outcome of a completed run.
type Result struct {
	// FinalReportJSON is the raw final fio JSON document (json+), stored verbatim
	// as the per-run summary. May be empty if fio failed before producing it.
	FinalReportJSON []byte
	// Summary is the distilled final metrics (best-effort; zero if unavailable).
	Summary SummaryMetrics
	// ExitCode is fio's process exit code.
	ExitCode int
}

// Runner executes a single fio job and streams live updates. It is single-use:
// create one per run. All process management funnels through startProcess/
// signalGroup/killGroup which are implemented per-platform so cleanup is correct
// on Linux (process groups) while the package still compiles elsewhere.
type Runner struct {
	// FioPath is the fio binary (default "fio", resolved on PATH).
	FioPath string

	mu      sync.Mutex
	proc    *managedProcess
	started bool
	stopped bool
}

// NewRunner creates a runner. fioPath may be empty to use "fio" from PATH.
func NewRunner(fioPath string) *Runner {
	if fioPath == "" {
		fioPath = "fio"
	}
	return &Runner{FioPath: fioPath}
}

// Run writes the jobfile into workDir, launches fio, streams live updates via
// onLive, and returns the final Result. It blocks until fio exits, the context
// is canceled, or Stop/Abort is called.
//
// workDir must be an absolute, agent-controlled directory; it is created if
// needed and fio's jobfile and logs live there.
func (r *Runner) Run(ctx context.Context, spec *Spec, workDir string, statusIntervalSec int, onLive func(LiveUpdate)) (*Result, error) {
	if statusIntervalSec < 1 {
		statusIntervalSec = 1
	}
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return nil, fmt.Errorf("create workDir: %w", err)
	}

	rendered, err := BuildJobfile(spec, workDir)
	if err != nil {
		return nil, err
	}
	jobfilePath := filepath.Join(workDir, "job.fio")
	if err := os.WriteFile(jobfilePath, []byte(rendered.JobfileContent), 0o640); err != nil {
		return nil, fmt.Errorf("write jobfile: %w", err)
	}

	// For file targets, materialize the test file to its full size *before* the
	// timed run. With time_based=1, fio counts the file-layout phase against the
	// measured runtime; a fresh read test would otherwise spend its whole runtime
	// creating the file and report ~0 IOPS. Pre-sizing makes the measurement
	// start on an existing, fully-allocated file. Best-effort: if this fails fio
	// still falls back to its own layout (via allow_file_create).
	if spec.TargetKind == TargetFile && spec.FileSizeMiB > 0 {
		if err := ensureFileSize(spec.TargetPath, spec.FileSizeMiB<<20); err != nil {
			// Non-fatal: log-less here (runner has no logger); fio will attempt its
			// own layout. We return the error only if the path is clearly unusable.
			if onLive == nil {
				_ = err // keep behavior identical in no-live callers
			}
		}
	}

	// We do NOT pass --output: with --status-interval, fio writes the periodic
	// status dumps AND the final report to its normal output. Routing that to a
	// file would send the interval dumps there too, leaving stdout empty and the
	// live view dead. So fio writes everything to stdout as a sequence of
	// concatenated JSON documents; we parse the stream for live updates and keep
	// the last complete document as the final report.
	args := []string{
		"--output-format=json+",
		fmt.Sprintf("--status-interval=%d", statusIntervalSec),
	}
	// Safety net for read-only workloads against a real block device: fio's
	// --readonly is a CLI flag (not a jobfile keyword) that makes fio refuse any
	// write. It is intentionally not used for file targets, where fio must create
	// its own test file first.
	if spec.WantsReadonlyGuard() {
		args = append(args, "--readonly")
	}
	// Live status also comes out as JSON; with --output set to a file, the
	// periodic status dumps go to stdout. We parse stdout for live data.
	args = append(args, jobfilePath)

	cmd := exec.CommandContext(ctx, r.FioPath, args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	mp := newManagedProcess(cmd)

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil, errors.New("runner already stopped")
	}
	r.proc = mp
	r.started = true
	r.mu.Unlock()

	if err := mp.start(); err != nil {
		return nil, fmt.Errorf("start fio: %w", err)
	}

	// Consume stdout for live JSON status documents in a goroutine. The last
	// complete document is captured as the final report.
	liveDone := make(chan struct{})
	var lastDoc []byte
	go func() {
		defer close(liveDone)
		lastDoc = r.consumeLive(stdout, onLive)
	}()

	// Wait for fio to exit (or context cancellation handled by CommandContext).
	waitErr := mp.wait()
	<-liveDone

	exitCode := mp.exitCode()

	res := &Result{ExitCode: exitCode}
	// The final report is the last complete JSON document fio emitted on stdout.
	if len(bytes.TrimSpace(lastDoc)) > 0 {
		res.FinalReportJSON = lastDoc
		if rep, pErr := ParseReport(lastDoc); pErr == nil {
			if sum, ok := rep.Summary(); ok {
				res.Summary = sum
			}
		}
	}

	if waitErr != nil {
		// A non-zero exit after a stop/abort is expected; distinguish it.
		r.mu.Lock()
		stopped := r.stopped
		r.mu.Unlock()
		if stopped {
			return res, nil
		}
		return res, fmt.Errorf("fio exited with error (code %d): %v: %s", exitCode, waitErr, stderrBuf.String())
	}
	return res, nil
}

// ensureFileSize makes sure the file at path exists and is at least sizeBytes
// large, creating parent directories as needed. It uses Truncate, which is fast
// and sparse-aware; the subsequent fallocate=native in the jobfile (and fio's
// first pass) back it with real blocks. This removes the file-layout phase from
// the measured, time_based runtime.
func ensureFileSize(path string, sizeBytes int64) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create target dir: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		return fmt.Errorf("open target file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat target file: %w", err)
	}
	if fi.Size() >= sizeBytes {
		return nil
	}
	if err := f.Truncate(sizeBytes); err != nil {
		return fmt.Errorf("truncate target file to %d bytes: %w", sizeBytes, err)
	}
	return nil
}

// consumeLive reads concatenated JSON status documents from fio's stdout and
// invokes onLive for each complete document. It returns the last complete
// document seen, which is the final report fio prints at exit.
func (r *Runner) consumeLive(stdout io.Reader, onLive func(LiveUpdate)) []byte {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	var acc []byte
	var last []byte
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			docs, rem := SplitConcatenatedJSON(acc)
			acc = rem
			for _, doc := range docs {
				last = doc
				if onLive == nil {
					continue
				}
				rep, pErr := ParseReport(doc)
				if pErr != nil {
					continue
				}
				if sum, ok := rep.Summary(); ok {
					onLive(LiveUpdate{Metrics: sum, At: time.Now()})
				}
			}
		}
		if err != nil {
			return last
		}
	}
}

// Stop requests a graceful stop (SIGTERM to the process group). fio flushes and
// writes its final report on SIGTERM.
func (r *Runner) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.proc == nil {
		return nil
	}
	return r.proc.signalGroupTerm()
}

// Abort forcefully kills the process group (SIGKILL). Use when a graceful stop
// did not take effect.
func (r *Runner) Abort() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.proc == nil {
		return nil
	}
	return r.proc.killGroup()
}
