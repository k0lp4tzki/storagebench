// Command broker is the privilege-broker: the only storagebench component that
// runs with elevated privileges. It applies and reverts network faults (via tc)
// on behalf of the unprivileged agent, over a local Unix socket, enforcing a
// strict allowlist, argv-only execution, a watchdog auto-revert, and an audit
// log. See internal/broker for the full security model.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/storagebench/storagebench/internal/broker"
	"github.com/storagebench/storagebench/internal/logx"
	"github.com/storagebench/storagebench/internal/version"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "broker: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		socketPath  = flag.String("socket", "/run/storagebench/broker.sock", "Unix socket path to listen on")
		group       = flag.String("group", "storagebench", "group name or gid allowed to connect (socket is group 0660)")
		auditPath   = flag.String("audit", "/var/log/storagebench/broker-audit.log", "append-only audit log path")
		logLevel    = flag.String("log-level", "info", "log level: debug|info|warn|error")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("storagebench-broker " + version.String())
		return nil
	}

	log := logx.New(*logLevel)

	audit, err := broker.OpenAuditLog(*auditPath)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer audit.Close()

	ln, err := broker.Listen(*socketPath, *group)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b := broker.New(log, audit)
	log.Info("privilege-broker listening", "socket", *socketPath, "group", *group)
	return b.Serve(ctx, ln)
}
