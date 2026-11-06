//go:build linux

package benchmark

import (
	"os/exec"
	"sync"
	"syscall"
)

// managedProcess wraps an *exec.Cmd and manages it as its own process group so
// that a stop/abort reliably reaches fio and any children, leaving no orphaned
// benchmark processes behind.
type managedProcess struct {
	cmd *exec.Cmd

	mu       sync.Mutex
	waited   bool
	exitCode_ int
}

func newManagedProcess(cmd *exec.Cmd) *managedProcess {
	// Place the child in a new process group (pgid = child pid) so signals can be
	// delivered to the whole group with a negative pid.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return &managedProcess{cmd: cmd, exitCode_: -1}
}

func (m *managedProcess) start() error {
	return m.cmd.Start()
}

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

// pgid returns the process group id (equal to the child pid since Setpgid).
func (m *managedProcess) pgid() int {
	if m.cmd.Process == nil {
		return 0
	}
	return m.cmd.Process.Pid
}

// signalGroupTerm sends SIGTERM to the whole process group.
func (m *managedProcess) signalGroupTerm() error {
	pgid := m.pgid()
	if pgid <= 0 {
		return nil
	}
	// Negative pid targets the process group.
	return syscall.Kill(-pgid, syscall.SIGTERM)
}

// killGroup sends SIGKILL to the whole process group.
func (m *managedProcess) killGroup() error {
	pgid := m.pgid()
	if pgid <= 0 {
		return nil
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
