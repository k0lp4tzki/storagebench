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

	// Final report path (json+). We ask fio to also emit live status as JSON to
	// stdout via --status-interval.
	finalReportPath := filepath.Join(workDir, "result.json")

	args := []string{
		"--output-format=json+",
		"--output=" + finalReportPath,
		fmt.Sprintf("--status-interval=%d", statusIntervalSec),
		// Live status also comes out as JSON; with --output set to a file, the
		// periodic status dumps go to stdout. We parse stdout for live data.
		jobfilePath,
	}

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

	// Consume stdout for live JSON status documents in a goroutine.
	liveDone := make(chan struct{})
	go func() {
		defer close(liveDone)
		r.consumeLive(stdout, onLive)
	}()

	// Wait for fio to exit (or context cancellation handled by CommandContext).
	waitErr := mp.wait()
	<-liveDone

	exitCode := mp.exitCode()

	res := &Result{ExitCode: exitCode}
	// Read the final report if fio wrote it.
	if data, rErr := os.ReadFile(finalReportPath); rErr == nil && len(bytes.TrimSpace(data)) > 0 {
		res.FinalReportJSON = data
		if rep, pErr := ParseReport(data); pErr == nil {
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

// consumeLive reads concatenated JSON status documents from fio's stdout and
// invokes onLive for each complete document.
func (r *Runner) consumeLive(stdout io.Reader, onLive func(LiveUpdate)) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	var acc []byte
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			docs, rem := SplitConcatenatedJSON(acc)
			acc = rem
			for _, doc := range docs {
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
			return
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
