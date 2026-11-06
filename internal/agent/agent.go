// Package agent implements the lightweight node daemon. It connects to the
// controller over WebSocket, authenticates with a pre-shared token, advertises
// its capabilities, answers heartbeats, and streams host telemetry.
//
// Resilience is a first-class concern: a controller disconnect must not abort a
// running test. The agent reconnects with exponential backoff and (in later
// phases) continues an already-authorized test locally while buffering
// telemetry for replay after reconnection. This file establishes the connection
// lifecycle; the benchmark execution plugs into the same structure.
package agent

import (
	"context"
	"crypto/tls"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"

	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/config"
	"github.com/storagebench/storagebench/internal/protocol"
	"github.com/storagebench/storagebench/internal/sysinfo"
	"github.com/storagebench/storagebench/internal/wsconn"
)

// osHostname returns the system hostname, or "unknown-host" if it cannot be
// determined. Used as a fallback agent ID.
func osHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

// Agent is the node daemon.
type Agent struct {
	cfg  config.Agent
	log  *slog.Logger
	id   string
	caps protocol.Capabilities
	sys  protocol.SystemInfo

	// sampler produces host metrics; wired in Run.
	sampler *Sampler

	// testCtl owns the fio benchmark lifecycle for this agent. Its Reporter is
	// set per-connection so results route to the active session.
	testCtl *TestController
}

// New constructs an Agent, probing system info and capabilities once at startup.
func New(cfg config.Agent, log *slog.Logger) *Agent {
	hostname := cfg.AgentID
	if hostname == "" {
		hostname = osHostname()
	}
	sys := sysinfo.Collect(hostname)
	caps := sysinfo.DetectCapabilities()

	id := cfg.AgentID
	if id == "" {
		id = hostname
	}

	sampler := NewSampler(time.Duration(cfg.SampleIntervalMS) * time.Millisecond)
	a := &Agent{
		cfg:     cfg,
		log:     log.With("agent", id),
		id:      id,
		caps:    caps,
		sys:     sys,
		sampler: sampler,
		testCtl: NewTestController(log.With("agent", id), sampler, cfg.SpoolDir, "fio"),
	}
	return a
}

// Run connects to the controller and maintains the connection until ctx is
// canceled, reconnecting with exponential backoff on failure.
func (a *Agent) Run(ctx context.Context) error {
	a.log.Info("agent starting",
		"controller", a.cfg.ControllerURL,
		"distro", a.sys.Distro,
		"kernel", a.sys.KernelVers,
		"fio", a.caps.FioVersion,
		"ioEngines", a.caps.IoEngines,
	)
	if a.cfg.InsecureSkipVerify {
		a.log.Warn("TLS certificate verification is DISABLED (insecureSkipVerify=true)")
	}
	if !a.caps.Fio {
		a.log.Warn("fio not found on PATH; this node cannot run benchmarks until fio is installed")
	}

	// Start host-metric sampling once; it runs independently of the connection
	// so telemetry keeps accumulating (and is replayed) across reconnects.
	go a.sampler.Start(ctx)

	backoff := time.Duration(a.cfg.ReconnectMinMS) * time.Millisecond
	maxBackoff := time.Duration(a.cfg.ReconnectMaxMS) * time.Millisecond

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		start := time.Now()
		err := a.connectOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			a.log.Warn("connection ended", "err", err)
		} else {
			a.log.Info("connection closed")
		}

		// If the connection lasted a while, reset backoff; otherwise grow it.
		if time.Since(start) > 2*maxBackoff {
			backoff = time.Duration(a.cfg.ReconnectMinMS) * time.Millisecond
		}

		sleep := jitter(backoff)
		a.log.Info("reconnecting", "in", sleep.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectOnce establishes a single connection and runs it until it drops.
func (a *Agent) connectOnce(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{}
	if a.cfg.InsecureSkipVerify {
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in for self-signed test certs
			},
		}
	}

	ws, _, err := websocket.Dial(dialCtx, a.cfg.ControllerURL, opts)
	if err != nil {
		return err
	}
	conn := wsconn.New(ws)
	defer func() { _ = conn.Close("bye") }()

	session := newSession(a, conn)
	return session.run(ctx)
}

// jitter applies +/-20% randomization to a backoff duration to avoid
// thundering-herd reconnects when the controller restarts.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	delta := float64(d) * 0.2
	return d + time.Duration((rand.Float64()*2-1)*delta)
}

// Hello builds the handshake frame advertising identity and capabilities.
func (a *Agent) hello() protocol.Hello {
	return protocol.Hello{
		ProtocolVersion: protocol.ProtocolVersion,
		Token:           a.cfg.Token,
		AgentID:         a.id,
		System:          a.sys,
		Capabilities:    a.caps,
		MonoNanos:       clock.MonoNanos(),
	}
}
