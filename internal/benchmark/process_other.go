//go:build !linux

package benchmark

import (
	"os/exec"
	"sync"
)

// managedProcess is the portable fallback used on non-Linux platforms so the
// package compiles on a developer workstation. It does not implement process
// groups; stop/abort fall back to killing the single process. The real runtime
// target is Linux (process_linux.go).
type managedProcess struct {
	cmd *exec.Cmd

	mu        sync.Mutex
	waited    bool
	exitCode_ int
}

func newManagedProcess(cmd *exec.Cmd) *managedProcess {
	return &managedProcess{cmd: cmd, exitCode_: -1}
}

func (m *managedProcess) start() error { return m.cmd.Start() }

func (m *managedProcess) wait() error {
	err := m.cmd.Wait()
	m.mu.Lock()
	m.waited = true
	if m.cmd.ProcessState != nil {
		m.exitCode_ = m.cmd.ProcessState.ExitCode()
	}
	m.mu.Unlock()
	return err
}

func (m *managedProcess) exitCode() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exitCode_
}

func (m *managedProcess) signalGroupTerm() error {
	if m.cmd.Process == nil {
		return nil
	}
	// No process-group support here; terminate the single process.
	return m.cmd.Process.Kill()
}

func (m *managedProcess) killGroup() error {
	if m.cmd.Process == nil {
		return nil
	}
	return m.cmd.Process.Kill()
}
