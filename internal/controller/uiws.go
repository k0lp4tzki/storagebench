package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/storagebench/storagebench/internal/wsconn"
)

// agentSnapshotInterval is how often a full agent-status snapshot is pushed to
// browsers, independent of live metric traffic. It keeps the dashboard's
// online/offline/RTT state fresh even when no test is running.
const agentSnapshotInterval = 3 * time.Second

// handleUIWebSocket upgrades a browser connection and streams live updates:
// agent-status snapshots plus broadcast metrics/events/test-state. The channel
// is push-only; inbound frames are read solely to detect disconnects.
func (s *Server) handleUIWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.broadcaster == nil {
		http.Error(w, "live stream unavailable", http.StatusServiceUnavailable)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Same-origin by default; the UI is served from this same origin.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.log.Debug("ui ws upgrade failed", "err", err)
		return
	}
	conn := wsconn.NewRaw(ws)
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "bye") }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client, unsubscribe := s.broadcaster.subscribe(256)
	defer unsubscribe()

	// Reader goroutine: detect client disconnect (and drain any frames).
	go func() {
		for {
			if _, _, err := ws.Read(ctx); err != nil {
				cancel()
				return
			}
		}
	}()

	// Push an immediate agent snapshot so the UI paints instantly.
	s.pushAgents(ctx, conn)

	ticker := time.NewTicker(agentSnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pushAgents(ctx, conn)
		case msg, ok := <-client.ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ctx, msg); err != nil {
				return
			}
		}
	}
}

// pushAgents sends a current agent-status snapshot as a UI message.
func (s *Server) pushAgents(ctx context.Context, conn *wsconn.RawConn) {
	raw, err := json.Marshal(s.hub.Snapshot())
	if err != nil {
		return
	}
	msg := UIMessage{Type: UITypeAgents, Payload: raw}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = conn.WriteJSON(wctx, msg)
}
