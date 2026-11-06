package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/protocol"
	"github.com/storagebench/storagebench/internal/wsconn"
)

// session owns one live connection to the controller.
type session struct {
	a    *Agent
	conn *wsconn.Conn

	// send serializes all writes to the controller through writeLoop.
	send chan protocol.Envelope

	heartbeat time.Duration
}

func newSession(a *Agent, conn *wsconn.Conn) *session {
	return &session{
		a:         a,
		conn:      conn,
		send:      make(chan protocol.Envelope, 128),
		heartbeat: 5 * time.Second, // refined by Welcome
	}
}

// run performs the handshake then drives read/write loops until the connection
// ends or ctx is canceled.
func (s *session) run(ctx context.Context) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := s.handshake(connCtx); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	s.a.log.Info("connected to controller", "heartbeat", s.heartbeat)

	errc := make(chan error, 3)

	go func() { errc <- s.writeLoop(connCtx) }()
	go func() { errc <- s.readLoop(connCtx) }()
	go func() { errc <- s.telemetryLoop(connCtx) }()

	// Return as soon as any loop exits; cancel the rest.
	err := <-errc
	cancel()
	return err
}

// handshake sends Hello and waits for Welcome (or Reject).
func (s *session) handshake(ctx context.Context) error {
	helloEnv, err := protocol.NewEnvelope(protocol.ChannelControl, protocol.TypeHello, clock.NowWallMilli(), s.a.hello())
	if err != nil {
		return err
	}
	if err := s.conn.WriteWithTimeout(ctx, 10*time.Second, helloEnv); err != nil {
		return err
	}

	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	env, err := s.conn.ReadEnvelope(readCtx)
	if err != nil {
		return err
	}
	switch env.Type {
	case protocol.TypeWelcome:
		var w protocol.Welcome
		if err := env.Decode(&w); err != nil {
			return err
		}
		if w.HeartbeatSeconds > 0 {
			s.heartbeat = time.Duration(w.HeartbeatSeconds) * time.Second
		}
		return nil
	case protocol.TypeReject:
		var rj protocol.Reject
		_ = env.Decode(&rj)
		return fmt.Errorf("rejected by controller: %s", rj.Reason)
	default:
		return fmt.Errorf("unexpected handshake reply: %s", env.Type)
	}
}

// writeLoop serializes outgoing frames.
func (s *session) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case env := <-s.send:
			if err := s.conn.WriteWithTimeout(ctx, 10*time.Second, env); err != nil {
				return fmt.Errorf("write: %w", err)
			}
		}
	}
}

// readLoop processes incoming control frames (pings, commands).
func (s *session) readLoop(ctx context.Context) error {
	for {
		env, err := s.conn.ReadEnvelope(ctx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if env.Channel != protocol.ChannelControl {
			// Agents currently only receive control frames.
			continue
		}
		switch env.Type {
		case protocol.TypePing:
			var p protocol.Ping
			_ = env.Decode(&p)
			pong := protocol.Pong{Nonce: p.Nonce, MonoNanos: clock.MonoNanos()}
			s.enqueueControl(protocol.TypePong, pong)
		case protocol.TypeAck:
			var ack protocol.Ack
			if err := env.Decode(&ack); err == nil {
				s.a.sampler.Acknowledge(ack.RunID, ack.Seq)
			}
		case protocol.TypeAssignTest, protocol.TypeStart, protocol.TypeStop, protocol.TypeAbort:
			// Benchmark lifecycle is handled in the next phase. For now we log
			// receipt so the control path is observable end-to-end.
			s.a.log.Info("received test command", "type", env.Type)
		default:
			s.a.log.Debug("unhandled control frame", "type", env.Type)
		}
	}
}

// telemetryLoop drains sampled metrics and ships them in batches.
func (s *session) telemetryLoop(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			batch, ok := s.a.sampler.DrainBatch()
			if !ok {
				continue
			}
			env, err := protocol.NewEnvelope(protocol.ChannelTelemetry, protocol.TypeMetricBatch, clock.NowWallMilli(), batch)
			if err != nil {
				continue
			}
			if err := s.enqueue(env); err != nil {
				return err
			}
		}
	}
}

func (s *session) enqueueControl(t protocol.MessageType, payload any) {
	env, err := protocol.NewEnvelope(protocol.ChannelControl, t, clock.NowWallMilli(), payload)
	if err != nil {
		return
	}
	_ = s.enqueue(env)
}

func (s *session) enqueue(env protocol.Envelope) error {
	select {
	case s.send <- env:
		return nil
	default:
		// The send buffer is full (controller slow/stalled). We drop telemetry
		// rather than block the whole session; the sampler retains unacked data
		// for later replay, so nothing is permanently lost.
		if env.Channel == protocol.ChannelControl {
			return errors.New("control send buffer full")
		}
		return nil
	}
}
