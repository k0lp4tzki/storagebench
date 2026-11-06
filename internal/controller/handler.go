package controller

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/storagebench/storagebench/internal/auth"
	"github.com/storagebench/storagebench/internal/clock"
	"github.com/storagebench/storagebench/internal/protocol"
	"github.com/storagebench/storagebench/internal/wsconn"
)

// handshakeTimeout bounds how long we wait for a valid Hello before dropping a
// new connection. This limits resource use from clients that connect but never
// authenticate.
const handshakeTimeout = 15 * time.Second

// AgentWSHandler returns an http.Handler that upgrades agent connections and
// drives their lifecycle. token is the expected pre-shared token; heartbeat is
// the ping interval.
func (h *Hub) AgentWSHandler(expectedToken string, heartbeat time.Duration, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// The agent is a trusted first-party client, not a browser, so we do
			// not need CORS origin checks here. TLS + token provide auth.
			InsecureSkipVerify: true,
		})
		if err != nil {
			log.Warn("agent ws upgrade failed", "err", err, "remote", r.RemoteAddr)
			return
		}
		conn := wsconn.New(ws)

		// Read the Hello within the handshake timeout.
		hsCtx, cancel := context.WithTimeout(r.Context(), handshakeTimeout)
		env, err := conn.ReadEnvelope(hsCtx)
		cancel()
		if err != nil {
			log.Warn("no hello received", "err", err, "remote", r.RemoteAddr)
			_ = conn.CloseError("handshake timeout")
			return
		}
		if env.Type != protocol.TypeHello {
			_ = conn.CloseError("expected hello")
			return
		}
		var hello protocol.Hello
		if err := env.Decode(&hello); err != nil {
			_ = conn.CloseError("malformed hello")
			return
		}

		// Validate protocol version (major component) and token.
		if hello.ProtocolVersion != protocol.ProtocolVersion {
			_ = sendReject(r.Context(), conn, "protocol version mismatch")
			return
		}
		if !auth.TokenValid(expectedToken, hello.Token) {
			log.Warn("agent auth rejected", "agent", hello.AgentID, "remote", r.RemoteAddr)
			_ = sendReject(r.Context(), conn, "authentication failed")
			return
		}
		if hello.AgentID == "" {
			_ = sendReject(r.Context(), conn, "empty agentId")
			return
		}

		ac := newAgentConn(hello.AgentID, conn, log)
		ac.setIdentity(hello, env.TSWallMilli)

		// Register (closing any stale duplicate connection).
		if old := h.register(ac); old != nil {
			log.Info("replacing existing agent connection", "agent", hello.AgentID)
			old.close()
		}
		h.telemetrySink.OnAgentHello(hello.AgentID, hello)
		log.Info("agent connected",
			"agent", hello.AgentID,
			"distro", hello.System.Distro,
			"kernel", hello.System.KernelVers,
			"fio", hello.Capabilities.FioVersion,
		)

		// Send Welcome.
		welcome := protocol.Welcome{
			ControllerWallMilli: clock.NowWallMilli(),
			HeartbeatSeconds:    int(heartbeat / time.Second),
			SessionID:           hello.AgentID + "-" + time.Now().UTC().Format("20060102150405"),
		}
		if err := ac.sendControl(protocol.TypeWelcome, welcome); err != nil {
			ac.close()
			h.unregister(ac)
			return
		}

		// Run write and read loops. The connection lives until either loop ends.
		connCtx, cancelConn := context.WithCancel(context.Background())
		defer cancelConn()

		go ac.writeLoop(connCtx, heartbeat)
		h.readLoop(connCtx, ac)

		// Cleanup on exit.
		ac.close()
		h.unregister(ac)
		_ = conn.Close("bye")
		log.Info("agent disconnected", "agent", hello.AgentID)
	}
}

// readLoop consumes envelopes from an agent until the connection closes.
func (h *Hub) readLoop(ctx context.Context, ac *agentConn) {
	for {
		if ac.isClosed() {
			return
		}
		env, err := ac.conn.ReadEnvelope(ctx)
		if err != nil {
			ac.log.Debug("agent read ended", "err", err)
			return
		}
		ac.lastSeenMs.Store(clock.NowWallMilli())

		switch env.Channel {
		case protocol.ChannelControl:
			h.handleControl(ac, env)
		case protocol.ChannelTelemetry:
			h.handleTelemetry(ac, env)
		default:
			ac.log.Warn("unknown channel", "ch", env.Channel)
		}
	}
}

func (h *Hub) handleControl(ac *agentConn, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypePong:
		var pong protocol.Pong
		if err := env.Decode(&pong); err == nil {
			// RTT: now (mono) minus the nonce we sent (a mono timestamp).
			rtt := time.Duration(clock.MonoNanos() - pong.Nonce)
			if rtt > 0 {
				ac.rttMs.Store(rtt.Milliseconds())
			}
		}
	case protocol.TypeTestState:
		var st protocol.TestState
		if err := env.Decode(&st); err == nil {
			h.telemetrySink.OnTestState(ac.agentID, st)
		}
	case protocol.TypeTestResult:
		var res protocol.TestResult
		if err := env.Decode(&res); err == nil {
			h.telemetrySink.OnTestResult(ac.agentID, res)
		}
	case protocol.TypeHello:
		// Ignore a duplicate hello on an established connection.
	default:
		ac.log.Debug("unhandled control type", "type", env.Type)
	}
}

func (h *Hub) handleTelemetry(ac *agentConn, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypeMetricBatch:
		var batch protocol.MetricBatch
		if err := env.Decode(&batch); err != nil {
			ac.log.Warn("bad metric batch", "err", err)
			return
		}
		h.telemetrySink.IngestMetrics(ac.agentID, batch)
		// Acknowledge up to this sequence so the agent can release its buffer.
		_ = ac.sendControl(protocol.TypeAck, protocol.Ack{RunID: batch.RunID, Seq: batch.Seq})
	case protocol.TypeEvent:
		var ev protocol.Event
		if err := env.Decode(&ev); err != nil {
			ac.log.Warn("bad event", "err", err)
			return
		}
		h.telemetrySink.IngestEvent(ac.agentID, ev)
	default:
		ac.log.Debug("unhandled telemetry type", "type", env.Type)
	}
}

func sendReject(ctx context.Context, conn *wsconn.Conn, reason string) error {
	env, err := protocol.NewEnvelope(protocol.ChannelControl, protocol.TypeReject, clock.NowWallMilli(), protocol.Reject{Reason: reason})
	if err == nil {
		wctx, cancel := context.WithTimeout(ctx, writeTimeout)
		_ = conn.WriteEnvelope(wctx, env)
		cancel()
	}
	return conn.CloseError(reason)
}
