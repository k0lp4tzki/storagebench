// Package wsconn wraps a WebSocket connection with JSON-envelope read/write
// helpers and sensible limits. Both the controller and the agent use it so the
// framing behavior is identical on both ends.
//
// We use github.com/coder/websocket (a small, context-aware, dependency-free
// WebSocket library) rather than a heavier framework. All reads and writes are
// context-bounded so a stalled peer cannot block a goroutine forever.
package wsconn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coder/websocket"

	"github.com/storagebench/storagebench/internal/protocol"
)

// maxMessageBytes caps a single incoming frame. Telemetry is batched but still
// bounded; this protects against a misbehaving or malicious peer.
const maxMessageBytes = 8 << 20 // 8 MiB

// Conn is a thin wrapper around a websocket.Conn that reads and writes
// protocol.Envelope values as JSON.
type Conn struct {
	ws *websocket.Conn
}

// New wraps an established websocket connection.
func New(ws *websocket.Conn) *Conn {
	ws.SetReadLimit(maxMessageBytes)
	return &Conn{ws: ws}
}

// WriteEnvelope marshals and sends an envelope using the provided context for
// deadline/cancellation.
func (c *Conn) WriteEnvelope(ctx context.Context, env protocol.Envelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	if err := c.ws.Write(ctx, websocket.MessageText, b); err != nil {
		return fmt.Errorf("ws write: %w", err)
	}
	return nil
}

// WriteWithTimeout is a convenience wrapper that bounds a write with a timeout
// derived from the parent context.
func (c *Conn) WriteWithTimeout(parent context.Context, timeout time.Duration, env protocol.Envelope) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return c.WriteEnvelope(ctx, env)
}

// ReadEnvelope blocks until the next envelope arrives or the context is done.
func (c *Conn) ReadEnvelope(ctx context.Context) (protocol.Envelope, error) {
	typ, data, err := c.ws.Read(ctx)
	if err != nil {
		return protocol.Envelope{}, err
	}
	if typ != websocket.MessageText {
		return protocol.Envelope{}, fmt.Errorf("unexpected ws message type %v", typ)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return protocol.Envelope{}, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return env, nil
}

// Close closes the underlying connection with a normal status code.
func (c *Conn) Close(reason string) error {
	return c.ws.Close(websocket.StatusNormalClosure, reason)
}

// CloseError closes the connection with an application error status.
func (c *Conn) CloseError(reason string) error {
	return c.ws.Close(websocket.StatusInternalError, truncateReason(reason))
}

// truncateReason keeps close reasons within the WebSocket control-frame limit
// (123 bytes for the reason phrase).
func truncateReason(s string) string {
	const max = 120
	if len(s) > max {
		return s[:max]
	}
	return s
}
