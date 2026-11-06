// Package controller implements the central coordinator: it accepts agent
// WebSocket connections, tracks their liveness, ingests telemetry, and exposes
// state to the HTTP/REST and UI layers.
package controller

import (
	"sync"
	"time"

	"github.com/storagebench/storagebench/internal/protocol"
)

// AgentStatus is the controller's view of an agent's liveness.
type AgentStatus string

const (
	StatusOnline   AgentStatus = "online"
	StatusDegraded AgentStatus = "degraded" // missed some heartbeats
	StatusOffline  AgentStatus = "offline"  // connection lost
)

// AgentView is a snapshot of an agent's state for the API/UI. It is a plain
// value type so it can be serialized without holding locks.
type AgentView struct {
	AgentID      string                `json:"agentId"`
	Status       AgentStatus           `json:"status"`
	Connected    bool                  `json:"connected"`
	System       protocol.SystemInfo   `json:"system"`
	Capabilities protocol.Capabilities `json:"capabilities"`
	LastSeenMs   int64                 `json:"lastSeenMs"`
	RTTms        int64                 `json:"rttMs"`
	// ClockOffsetMs is the estimated wall-clock offset (agent - controller),
	// useful context when correlating timelines across hosts.
	ClockOffsetMs int64 `json:"clockOffsetMs"`
}

// Hub is the registry of connected agents. It is safe for concurrent use.
type Hub struct {
	mu     sync.RWMutex
	agents map[string]*agentConn

	// degradedAfter / offlineAfter translate missed heartbeats into status.
	degradedAfter time.Duration
	offlineAfter  time.Duration

	// telemetrySink receives ingested telemetry for persistence. It is invoked
	// without holding the hub lock.
	telemetrySink TelemetrySink
}

// TelemetrySink persists telemetry ingested from agents. The store implements
// this; the hub depends only on the interface to stay decoupled from storage.
type TelemetrySink interface {
	IngestMetrics(agentID string, batch protocol.MetricBatch)
	IngestEvent(agentID string, ev protocol.Event)
	// OnAgentHello records/updates an agent's registration (system info, caps).
	OnAgentHello(agentID string, h protocol.Hello)
	// OnTestState records a lifecycle transition reported by an agent.
	OnTestState(agentID string, st protocol.TestState)
	// OnTestResult records the final per-run result reported by an agent.
	OnTestResult(agentID string, res protocol.TestResult)
}

// NewHub creates a hub. heartbeat is the ping interval; an agent is marked
// degraded after missing ~2 intervals and offline after ~4.
func NewHub(heartbeat time.Duration, sink TelemetrySink) *Hub {
	return &Hub{
		agents:        make(map[string]*agentConn),
		degradedAfter: 2 * heartbeat,
		offlineAfter:  4 * heartbeat,
		telemetrySink: sink,
	}
}

// register adds or replaces an agent connection. If an agent with the same ID
// is already connected, the old connection is closed (last-writer-wins) to avoid
// duplicate test execution and split-brain telemetry.
func (h *Hub) register(ac *agentConn) (old *agentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	old = h.agents[ac.agentID]
	h.agents[ac.agentID] = ac
	return old
}

// unregister removes an agent connection if it is still the current one.
func (h *Hub) unregister(ac *agentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.agents[ac.agentID]; ok && cur == ac {
		delete(h.agents, ac.agentID)
	}
}

// get returns the current connection for an agent ID, if connected.
func (h *Hub) get(agentID string) (*agentConn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ac, ok := h.agents[agentID]
	return ac, ok
}

// Snapshot returns a view of all currently known agents.
func (h *Hub) Snapshot() []AgentView {
	h.mu.RLock()
	conns := make([]*agentConn, 0, len(h.agents))
	for _, ac := range h.agents {
		conns = append(conns, ac)
	}
	h.mu.RUnlock()

	views := make([]AgentView, 0, len(conns))
	for _, ac := range conns {
		views = append(views, ac.view(h.degradedAfter, h.offlineAfter))
	}
	return views
}

// SendControl sends a control-channel message to a specific agent. It returns
// ErrAgentNotConnected if the agent is not currently connected.
func (h *Hub) SendControl(agentID string, t protocol.MessageType, payload any) error {
	ac, ok := h.get(agentID)
	if !ok {
		return ErrAgentNotConnected
	}
	return ac.sendControl(t, payload)
}
