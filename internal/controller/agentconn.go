package controller

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/protocol"
	"github.com/storagebench/storagebench/internal/wsconn"
)

// ErrAgentNotConnected is returned when a control message targets an agent that
// has no live connection.
var ErrAgentNotConnected = errors.New("agent not connected")

// writeTimeout bounds a single control write so a wedged socket cannot block.
const writeTimeout = 10 * time.Second

// agentConn represents one connected agent from the controller's perspective.
// All writes go through a single goroutine via the send channel to keep the
// WebSocket writer single-threaded (required by the library).
type agentConn struct {
	agentID string
	conn    *wsconn.Conn
	log     *slog.Logger

	send chan protocol.Envelope

	// liveness/metrics, accessed atomically.
	lastSeenMs    atomic.Int64
	rttMs         atomic.Int64
	clockOffsetMs atomic.Int64

	// identity captured at Hello, guarded by mu.
	mu     sync.RWMutex
	system protocol.SystemInfo
	caps   protocol.Capabilities

	closeOnce sync.Once
	closed    chan struct{}
}

func newAgentConn(agentID string, conn *wsconn.Conn, log *slog.Logger) *agentConn {
	ac := &agentConn{
		agentID: agentID,
		conn:    conn,
		log:     log.With("agent", agentID),
		send:    make(chan protocol.Envelope, 64),
		closed:  make(chan struct{}),
	}
	ac.lastSeenMs.Store(clock.NowWallMilli())
	return ac
}

// setIdentity records the agent's advertised identity/capabilities and seeds
// the clock-offset estimate from the Hello envelope timestamp. agentWallMs is
// the wall-clock time the agent stamped on the Hello envelope.
func (ac *agentConn) setIdentity(h protocol.Hello, agentWallMs int64) {
	ac.mu.Lock()
	ac.system = h.System
	ac.caps = h.Capabilities
	ac.mu.Unlock()
	// Rough clock offset: agent wall time at hello minus controller now. NTP
	// keeps this near zero; the value is informational context for correlation.
	ac.clockOffsetMs.Store(agentWallMs - clock.NowWallMilli())
}

// view builds an immutable snapshot for the API/UI.
func (ac *agentConn) view(degradedAfter, offlineAfter time.Duration) AgentView {
	ac.mu.RLock()
	sys := ac.system
	caps := ac.caps
	ac.mu.RUnlock()

	lastSeen := ac.lastSeenMs.Load()
	age := time.Duration(clock.NowWallMilli()-lastSeen) * time.Millisecond
	status := StatusOnline
	switch {
	case age >= offlineAfter:
		status = StatusOffline
	case age >= degradedAfter:
		status = StatusDegraded
	}
	return AgentView{
		AgentID:       ac.agentID,
		Status:        status,
		Connected:     !ac.isClosed(),
		System:        sys,
		Capabilities:  caps,
		LastSeenMs:    lastSeen,
		RTTms:         ac.rttMs.Load(),
		ClockOffsetMs: ac.clockOffsetMs.Load(),
	}
}

// sendControl enqueues a control-channel message for delivery.
func (ac *agentConn) sendControl(t protocol.MessageType, payload any) error {
	env, err := protocol.NewEnvelope(protocol.ChannelControl, t, clock.NowWallMilli(), payload)
	if err != nil {
		return err
	}
	return ac.enqueue(env)
}

func (ac *agentConn) enqueue(env protocol.Envelope) error {
	select {
	case ac.send <- env:
		return nil
	case <-ac.closed:
		return ErrAgentNotConnected
	default:
		// Backpressure: the send buffer is full. Dropping control messages is
		// unsafe, so we fail loudly rather than silently losing a command.
		return errors.New("agent send buffer full")
	}
}

func (ac *agentConn) isClosed() bool {
	select {
	case <-ac.closed:
		return true
	default:
		return false
	}
}

func (ac *agentConn) close() {
	ac.closeOnce.Do(func() { close(ac.closed) })
}

// writeLoop serializes all writes to the socket and emits periodic pings.
func (ac *agentConn) writeLoop(ctx context.Context, heartbeat time.Duration) {
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ac.closed:
			return
		case env := <-ac.send:
			if err := ac.conn.WriteWithTimeout(ctx, writeTimeout, env); err != nil {
				ac.log.Warn("write failed, closing agent conn", "err", err)
				ac.close()
				return
			}
		case <-ticker.C:
			ping := protocol.Ping{Nonce: clock.MonoNanos()}
			env, err := protocol.NewEnvelope(protocol.ChannelControl, protocol.TypePing, clock.NowWallMilli(), ping)
			if err != nil {
				continue
			}
			if err := ac.conn.WriteWithTimeout(ctx, writeTimeout, env); err != nil {
				ac.log.Warn("ping write failed, closing agent conn", "err", err)
				ac.close()
				return
			}
		}
	}
}
