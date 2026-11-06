package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/storagebench/storagebench/internal/benchmark"
	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/protocol"
)

// Reporter is how the TestController reports lifecycle transitions, events, and
// the final result back toward the controller. The agent session implements it.
// Decoupling via an interface keeps the TestController testable and independent
// of the connection.
type Reporter interface {
	ReportState(st protocol.TestState)
	ReportEvent(ev protocol.Event)
	ReportResult(res protocol.TestResult)
}

// assignedTest is the on-disk record of an authorized job, so the agent can
// resume a test across a controller disconnect.
type assignedTest struct {
	RunID                string         `json:"runId"`
	AuthorizedUntilMilli int64          `json:"authorizedUntilMilli"`
	Spec                 benchmark.Spec `json:"spec"`
}

// TestController enforces one-active-test-per-node and owns the fio runner
// lifecycle for the agent. It is safe for concurrent use.
type TestController struct {
	log      *slog.Logger
	sampler  *Sampler
	spoolDir string
	fioPath  string

	// reporterMu guards reporter, which is swapped per connection. A running test
	// must not abort when the controller disconnects, so the reporter can be a
	// no-op during a gap; telemetry still accumulates in the sampler and is
	// replayed on reconnect.
	reporterMu sync.RWMutex
	reporter   Reporter

	mu      sync.Mutex
	current *assignedTest
	runner  *benchmark.Runner
	running bool
	cancel  context.CancelFunc
}

// nopReporter is used when no session is connected.
type nopReporter struct{}

func (nopReporter) ReportState(protocol.TestState)   {}
func (nopReporter) ReportEvent(protocol.Event)       {}
func (nopReporter) ReportResult(protocol.TestResult) {}

// NewTestController constructs a TestController. spoolDir is where authorized
// jobs and fio working files are stored.
func NewTestController(log *slog.Logger, sampler *Sampler, spoolDir, fioPath string) *TestController {
	return &TestController{
		log:      log,
		sampler:  sampler,
		spoolDir: spoolDir,
		fioPath:  fioPath,
		reporter: nopReporter{},
	}
}

// SetReporter swaps the active reporter (called when a session connects). Passing
// nil installs the no-op reporter.
func (tc *TestController) SetReporter(r Reporter) {
	tc.reporterMu.Lock()
	if r == nil {
		r = nopReporter{}
	}
	tc.reporter = r
	tc.reporterMu.Unlock()
}

// report returns the current reporter under the read lock.
func (tc *TestController) report() Reporter {
	tc.reporterMu.RLock()
	defer tc.reporterMu.RUnlock()
	return tc.reporter
}

// Assign records and persists an authorized test. It rejects a new assignment
// while a test is running (one active test per node).
func (tc *TestController) Assign(runID string, authorizedUntilMilli int64, specRaw json.RawMessage) error {
	var spec benchmark.Spec
	if err := json.Unmarshal(specRaw, &spec); err != nil {
		return fmt.Errorf("decode spec: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.running {
		return fmt.Errorf("a test is already running")
	}

	at := &assignedTest{RunID: runID, AuthorizedUntilMilli: authorizedUntilMilli, Spec: spec}
	if err := tc.persist(at); err != nil {
		return fmt.Errorf("persist assignment: %w", err)
	}
	tc.current = at
	tc.reportState(runID, protocol.PhaseAssigned, "assigned")
	tc.log.Info("test assigned", "run", runID, "workload", spec.Workload, "target", spec.TargetPath)
	return nil
}

// Start launches the currently assigned test with the given run id. It returns
// an error if no matching test is assigned or one is already running.
func (tc *TestController) Start(runID string) error {
	tc.mu.Lock()
	if tc.current == nil || tc.current.RunID != runID {
		tc.mu.Unlock()
		return fmt.Errorf("no assigned test with run id %q", runID)
	}
	if tc.running {
		tc.mu.Unlock()
		return fmt.Errorf("a test is already running")
	}
	// Reject starting an authorization that has already expired.
	if tc.current.AuthorizedUntilMilli > 0 && clock.NowWallMilli() > tc.current.AuthorizedUntilMilli {
		tc.mu.Unlock()
		return fmt.Errorf("authorization for run %q has expired", runID)
	}

	spec := tc.current.Spec
	runner := benchmark.NewRunner(tc.fioPath)
	runCtx, cancel := context.WithCancel(context.Background())
	tc.runner = runner
	tc.cancel = cancel
	tc.running = true
	tc.mu.Unlock()

	workDir := filepath.Join(tc.spoolDir, "runs", sanitizeID(runID))

	// Associate telemetry with this run and begin merging fio metrics.
	tc.sampler.SetRunID(runID)

	go tc.execute(runCtx, runID, &spec, runner, workDir)
	return nil
}

// execute runs fio to completion and reports state + result. It always clears
// the running flag and fio-metric merging on exit.
func (tc *TestController) execute(ctx context.Context, runID string, spec *benchmark.Spec, runner *benchmark.Runner, workDir string) {
	defer func() {
		tc.mu.Lock()
		tc.running = false
		tc.runner = nil
		tc.cancel = nil
		tc.mu.Unlock()
		tc.sampler.ClearFioMetrics()
		tc.sampler.SetRunID("")
	}()

	tc.reportState(runID, protocol.PhaseStarting, "launching fio")
	tc.reportEvent(runID, protocol.SeverityInfo, "fio", "benchmark starting")

	onLive := func(u benchmark.LiveUpdate) {
		tc.sampler.SetFioMetrics(protocol.MetricSample{
			IOPSRead:   u.Metrics.IOPSRead,
			IOPSWrite:  u.Metrics.IOPSWrite,
			BWReadKBs:  u.Metrics.BWReadKBs,
			BWWriteKBs: u.Metrics.BWWriteKBs,
			LatMeanUs:  u.Metrics.LatMeanUs,
			LatP95Us:   u.Metrics.LatP95Us,
			LatP99Us:   u.Metrics.LatP99Us,
		})
	}

	tc.reportState(runID, protocol.PhaseRunning, "running")
	res, err := runner.Run(ctx, spec, workDir, 1, onLive)
	if err != nil {
		tc.log.Warn("fio run failed", "run", runID, "err", err)
		tc.reportEvent(runID, protocol.SeverityError, "fio", err.Error())
		tc.reportState(runID, protocol.PhaseFailed, err.Error())
		return
	}

	// Report the final result.
	result := protocol.TestResult{
		RunID:      runID,
		ExitCode:   res.ExitCode,
		IOPSRead:   res.Summary.IOPSRead,
		IOPSWrite:  res.Summary.IOPSWrite,
		BWReadKBs:  res.Summary.BWReadKBs,
		BWWriteKBs: res.Summary.BWWriteKBs,
		LatMeanUs:  res.Summary.LatMeanUs,
		LatP95Us:   res.Summary.LatP95Us,
		LatP99Us:   res.Summary.LatP99Us,
	}
	if len(res.FinalReportJSON) > 0 {
		result.FioReportJSON = string(res.FinalReportJSON)
	}
	tc.report().ReportResult(result)
	tc.reportState(runID, protocol.PhaseCompleted, "completed")
	tc.reportEvent(runID, protocol.SeverityInfo, "fio", "benchmark completed")
	tc.log.Info("test completed", "run", runID, "iopsR", result.IOPSRead, "iopsW", result.IOPSWrite)
}

// Stop requests a graceful stop of the running test.
func (tc *TestController) Stop(runID string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if !tc.running || tc.current == nil || tc.current.RunID != runID {
		return fmt.Errorf("no running test with run id %q", runID)
	}
	tc.reportState(runID, protocol.PhaseStopping, "stopping")
	if tc.runner != nil {
		return tc.runner.Stop()
	}
	return nil
}

// Abort forcefully aborts the running test and cancels its context.
func (tc *TestController) Abort(runID, reason string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if !tc.running || tc.current == nil || tc.current.RunID != runID {
		return fmt.Errorf("no running test with run id %q", runID)
	}
	tc.reportState(runID, protocol.PhaseAborting, reason)
	if tc.cancel != nil {
		tc.cancel()
	}
	if tc.runner != nil {
		return tc.runner.Abort()
	}
	return nil
}

// IsRunning reports whether a test is currently executing.
func (tc *TestController) IsRunning() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.running
}

// --- helpers ---

func (tc *TestController) reportState(runID string, phase protocol.TestStatePhase, msg string) {
	tc.report().ReportState(protocol.TestState{RunID: runID, Phase: phase, Message: msg})
}

func (tc *TestController) reportEvent(runID string, sev protocol.EventSeverity, src, msg string) {
	tc.report().ReportEvent(protocol.Event{
		RunID:       runID,
		TSWallMilli: clock.NowWallMilli(),
		MonoNanos:   clock.MonoNanos(),
		Severity:    sev,
		Source:      src,
		Message:     msg,
	})
}

// persist writes the authorized test to disk so it survives a restart and can be
// resumed across a controller disconnect.
func (tc *TestController) persist(at *assignedTest) error {
	if err := os.MkdirAll(tc.spoolDir, 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(at, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(tc.spoolDir, "current.json.tmp")
	final := filepath.Join(tc.spoolDir, "current.json")
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// sanitizeID makes a run id safe to use as a directory name. Run ids are
// controller-generated and already constrained, but we defend in depth.
func sanitizeID(id string) string {
	out := make([]rune, 0, len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return string(out)
}
