// Package broker implements the privilege-broker server: the only component in
// the system that runs with elevated privileges. It exists so the agent can stay
// fully unprivileged. It applies and reverts network faults via `tc`, enforcing:
//
//   - a strict program allowlist resolved to fixed absolute paths,
//   - argv execution (never a shell),
//   - server-side re-validation of every request,
//   - a watchdog that auto-reverts any fault whose TTL elapses, and
//   - an append-only audit log of every action and its outcome.
//
// It listens on a local Unix domain socket whose permissions restrict access to
// the storagebench group, so only the local agent can talk to it.
package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/storagebench/storagebench/internal/fault"
)

// programPaths maps allowlisted program names to their fixed absolute paths. The
// broker refuses anything not in this map and never searches PATH.
var programPaths = map[string]string{
	"tc": "/sbin/tc",
}

// altProgramPaths are fallback locations checked if the primary path is absent
// (distros differ: /sbin vs /usr/sbin).
var altProgramPaths = map[string][]string{
	"tc": {"/usr/sbin/tc", "/bin/tc", "/usr/bin/tc"},
}

// activeFault tracks one applied fault for the watchdog and revert-all sweep.
type activeFault struct {
	iface  string
	runID  string
	timer  *time.Timer
	expiry time.Time
}

// Broker is the privilege-broker server.
type Broker struct {
	log     *slog.Logger
	audit   *AuditLog
	cmdTO   time.Duration
	execCmd func(ctx context.Context, path string, args []string) ([]byte, error)

	mu     sync.Mutex
	active map[string]*activeFault // keyed by iface
}

// New creates a broker. auditPath is the append-only audit log file.
func New(log *slog.Logger, audit *AuditLog) *Broker {
	b := &Broker{
		log:    log,
		audit:  audit,
		cmdTO:  15 * time.Second,
		active: make(map[string]*activeFault),
	}
	b.execCmd = b.runProgram
	return b
}

// resolveProgram returns the absolute path for an allowlisted program name, or
// an error if the program is not allowlisted or cannot be found.
func resolveProgram(name string) (string, error) {
	if !fault.AllowedPrograms[name] {
		return "", fmt.Errorf("program %q is not allowlisted", name)
	}
	primary, ok := programPaths[name]
	if ok {
		if _, err := os.Stat(primary); err == nil {
			return primary, nil
		}
	}
	for _, p := range altProgramPaths[name] {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("program %q not found in any allowlisted path", name)
}

// runProgram executes an allowlisted program with the given args via exec (no
// shell), returning combined output. It enforces a timeout.
func (b *Broker) runProgram(ctx context.Context, name string, args []string) ([]byte, error) {
	path, err := resolveProgram(name)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, b.cmdTO)
	defer cancel()
	// #nosec G204 -- name is allowlisted and resolved to a fixed absolute path;
	// args are built from a validated spec (no shell, no user free-text).
	cmd := exec.CommandContext(cctx, path, args[1:]...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/bin:/usr/bin"}
	return cmd.CombinedOutput()
}

// execCommands runs a sequence of fault.Command argvs. The first entry of each
// Args is the program name. A command marked IgnoreError does not abort the
// sequence on failure.
func (b *Broker) execCommands(ctx context.Context, cmds []fault.Command) error {
	for _, c := range cmds {
		if len(c.Args) == 0 {
			return fmt.Errorf("empty command")
		}
		out, err := b.execCmd(ctx, c.Args[0], c.Args)
		if err != nil {
			if c.IgnoreError {
				b.log.Debug("ignored command error", "args", c.Args, "err", err, "out", string(out))
				continue
			}
			return fmt.Errorf("%v: %w: %s", c.Args, err, string(out))
		}
	}
	return nil
}

// Apply installs a fault and arms its watchdog. Any existing fault on the same
// interface is reverted first so there is always a single root qdisc.
func (b *Broker) Apply(ctx context.Context, spec fault.Spec, ttlSeconds int, runID string) error {
	if err := spec.Validate(); err != nil {
		b.audit.Record("apply", runID, spec.Iface, spec.Describe(), false, err.Error())
		return err
	}
	if ttlSeconds <= 0 {
		err := fmt.Errorf("ttlSeconds must be positive")
		b.audit.Record("apply", runID, spec.Iface, spec.Describe(), false, err.Error())
		return err
	}

	// Revert any prior fault on this interface first (single root qdisc).
	revert, _ := fault.RevertCommands(spec.Iface)
	_ = b.execCommands(ctx, revert)

	apply, err := spec.ApplyCommands()
	if err != nil {
		b.audit.Record("apply", runID, spec.Iface, spec.Describe(), false, err.Error())
		return err
	}
	if err := b.execCommands(ctx, apply); err != nil {
		// Best-effort cleanup if apply partially succeeded.
		_ = b.execCommands(ctx, revert)
		b.audit.Record("apply", runID, spec.Iface, spec.Describe(), false, err.Error())
		return err
	}

	b.arm(spec.Iface, runID, time.Duration(ttlSeconds)*time.Second)
	b.audit.Record("apply", runID, spec.Iface, spec.Describe(), true, "")
	b.log.Info("fault applied", "iface", spec.Iface, "kind", spec.Kind, "ttl", ttlSeconds, "run", runID)
	return nil
}

// arm (re)starts the watchdog timer for an interface.
func (b *Broker) arm(iface, runID string, ttl time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.active[iface]; ok && existing.timer != nil {
		existing.timer.Stop()
	}
	af := &activeFault{iface: iface, runID: runID, expiry: time.Now().Add(ttl)}
	af.timer = time.AfterFunc(ttl, func() { b.watchdogRevert(iface) })
	b.active[iface] = af
}

// watchdogRevert is invoked by the TTL timer; it reverts the fault and logs it.
func (b *Broker) watchdogRevert(iface string) {
	b.log.Warn("fault watchdog fired; auto-reverting", "iface", iface)
	if err := b.Revert(context.Background(), iface, "watchdog"); err != nil {
		b.log.Error("watchdog revert failed", "iface", iface, "err", err)
	}
}

// Revert removes the fault on an interface and disarms its watchdog.
func (b *Broker) Revert(ctx context.Context, iface, runID string) error {
	if err := fault.ValidateIface(iface); err != nil {
		b.audit.Record("revert", runID, iface, "", false, err.Error())
		return err
	}
	cmds, _ := fault.RevertCommands(iface)
	err := b.execCommands(ctx, cmds)

	b.mu.Lock()
	if af, ok := b.active[iface]; ok {
		if af.timer != nil {
			af.timer.Stop()
		}
		delete(b.active, iface)
	}
	b.mu.Unlock()

	if err != nil {
		b.audit.Record("revert", runID, iface, "", false, err.Error())
		return err
	}
	b.audit.Record("revert", runID, iface, "", true, "")
	b.log.Info("fault reverted", "iface", iface, "run", runID)
	return nil
}

// RevertAll reverts every tracked fault. Returns the list of interfaces handled.
func (b *Broker) RevertAll(ctx context.Context) []string {
	b.mu.Lock()
	ifaces := make([]string, 0, len(b.active))
	for iface := range b.active {
		ifaces = append(ifaces, iface)
	}
	b.mu.Unlock()
	for _, iface := range ifaces {
		_ = b.Revert(ctx, iface, "revert_all")
	}
	return ifaces
}

// Serve accepts connections on ln until ctx is canceled. On shutdown it reverts
// all tracked faults as a safety sweep.
func (b *Broker) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	defer func() {
		reverted := b.RevertAll(context.Background())
		if len(reverted) > 0 {
			b.log.Info("reverted faults on shutdown", "ifaces", reverted)
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go b.handleConn(ctx, conn)
	}
}

// handleConn processes newline-delimited JSON requests on one connection.
func (b *Broker) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	enc := json.NewEncoder(conn)

	for sc.Scan() {
		var req fault.BrokerRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = enc.Encode(fault.BrokerResponse{OK: false, Error: "bad request json"})
			continue
		}
		resp := b.dispatch(ctx, req)
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

// dispatch routes one request to the matching handler.
func (b *Broker) dispatch(ctx context.Context, req fault.BrokerRequest) fault.BrokerResponse {
	switch req.Action {
	case fault.ActionPing:
		return fault.BrokerResponse{OK: true}
	case fault.ActionApply:
		if err := b.Apply(ctx, req.Spec, req.TTLSeconds, req.RunID); err != nil {
			return fault.BrokerResponse{OK: false, Error: err.Error()}
		}
		return fault.BrokerResponse{OK: true}
	case fault.ActionRevert:
		if err := b.Revert(ctx, req.Spec.Iface, req.RunID); err != nil {
			return fault.BrokerResponse{OK: false, Error: err.Error()}
		}
		return fault.BrokerResponse{OK: true}
	case fault.ActionRevertAll:
		reverted := b.RevertAll(ctx)
		return fault.BrokerResponse{OK: true, Reverted: reverted}
	default:
		return fault.BrokerResponse{OK: false, Error: fmt.Sprintf("unknown action %q", req.Action)}
	}
}
