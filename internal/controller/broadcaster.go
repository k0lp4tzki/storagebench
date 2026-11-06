package controller

import (
	"encoding/json"
	"sync"

	"github.com/storagebench/storagebench/internal/protocol"
)

// UIMessage is a message pushed to browser clients over the UI WebSocket. The
// Type selects the payload shape; the frontend switches on it.
type UIMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// UI message types.
const (
	UITypeMetrics   = "metrics"   // live metric batch for a run+agent
	UITypeEvent     = "event"     // discrete event
	UITypeTestState = "testState" // run lifecycle transition
	UITypeAgents    = "agents"    // periodic agent status snapshot
)

// UIMetrics is the live metric payload sent to browsers. It mirrors the ingested
// batch but tags the originating agent so the UI can place it per-server.
type UIMetrics struct {
	AgentID string                  `json:"agentId"`
	RunID   string                  `json:"runId"`
	Samples []protocol.MetricSample `json:"samples"`
}

// UIEvent is the browser-facing event payload (agent-tagged).
type UIEvent struct {
	AgentID string         `json:"agentId"`
	Event   protocol.Event `json:"event"`
}

// UITestState is the browser-facing lifecycle payload (agent-tagged).
type UITestState struct {
	AgentID string             `json:"agentId"`
	State   protocol.TestState `json:"state"`
}

// Broadcaster fans out live updates to connected browser clients. It also
// implements TelemetrySink by wrapping the persistent sink (the store): every
// ingested item is first persisted, then broadcast. This keeps persistence
// authoritative while giving the UI a real-time view, with no extra coupling in
// the hub.
type Broadcaster struct {
	persistent TelemetrySink

	mu      sync.RWMutex
	clients map[*uiClient]struct{}
}

// uiClient is a single browser subscriber. Messages are delivered through a
// buffered channel; a slow client is dropped rather than allowed to block the
// ingestion path.
type uiClient struct {
	ch chan UIMessage
}

// NewBroadcaster wraps the persistent sink.
func NewBroadcaster(persistent TelemetrySink) *Broadcaster {
	return &Broadcaster{
		persistent: persistent,
		clients:    make(map[*uiClient]struct{}),
	}
}

// subscribe registers a new client and returns it plus an unsubscribe func.
func (b *Broadcaster) subscribe(buffer int) (*uiClient, func()) {
	c := &uiClient{ch: make(chan UIMessage, buffer)}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c, func() {
		b.mu.Lock()
		if _, ok := b.clients[c]; ok {
			delete(b.clients, c)
			close(c.ch)
		}
		b.mu.Unlock()
	}
}

// publish delivers a message to all clients without blocking on slow ones.
func (b *Broadcaster) publish(msg UIMessage) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.clients {
		select {
		case c.ch <- msg:
		default:
			// Client is too slow; drop this message for it. The next full agent
			// snapshot will re-sync its high-level state.
		}
	}
}

// Publish marshals and broadcasts a typed message. Marshalling failures are
// silently ignored (they indicate a programming error, not a runtime condition).
func (b *Broadcaster) Publish(msgType string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	b.publish(UIMessage{Type: msgType, Payload: raw})
}

// --- TelemetrySink (decorator over the persistent store) ---

func (b *Broadcaster) IngestMetrics(agentID string, batch protocol.MetricBatch) {
	b.persistent.IngestMetrics(agentID, batch)
	b.Publish(UITypeMetrics, UIMetrics{AgentID: agentID, RunID: batch.RunID, Samples: batch.Samples})
}

func (b *Broadcaster) IngestEvent(agentID string, ev protocol.Event) {
	b.persistent.IngestEvent(agentID, ev)
	b.Publish(UITypeEvent, UIEvent{AgentID: agentID, Event: ev})
}

func (b *Broadcaster) OnAgentHello(agentID string, h protocol.Hello) {
	b.persistent.OnAgentHello(agentID, h)
}

func (b *Broadcaster) OnTestState(agentID string, st protocol.TestState) {
	b.persistent.OnTestState(agentID, st)
	b.Publish(UITypeTestState, UITestState{AgentID: agentID, State: st})
}

func (b *Broadcaster) OnTestResult(agentID string, res protocol.TestResult) {
	b.persistent.OnTestResult(agentID, res)
	// The result is reflected via the subsequent test_state=completed broadcast
	// and is fetchable via REST; no separate large-payload broadcast needed.
}
