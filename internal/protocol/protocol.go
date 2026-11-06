// Package protocol defines the wire contract shared by the controller and the
// agents. Everything that crosses the WebSocket connection is a Envelope.
//
// Design notes:
//   - A single WebSocket connection carries two logical channels: "control"
//     (commands, lifecycle, heartbeats) and "telemetry" (metric batches,
//     events, fio progress). This satisfies the requirement to keep control
//     and data logically separated while using one authenticated connection.
//   - Every message carries monotonic-friendly timestamps so that samples from
//     three different hosts can be placed on a common timeline. Wall-clock time
//     (UnixMilli, UTC) is used for cross-node correlation; agents additionally
//     report a monotonic nanosecond counter for drift-free durations.
//   - Messages are intentionally small and JSON-encoded for the control
//     channel. Telemetry is sent in batches to keep per-sample overhead low so
//     the measurement machinery does not perturb the benchmark itself.
package protocol

import "encoding/json"

// ProtocolVersion is bumped on breaking changes to the wire format. The agent
// and controller exchange it during the handshake and refuse to proceed on a
// mismatch of the major component.
const ProtocolVersion = "1"

// Channel identifies the logical channel a message belongs to.
type Channel string

const (
	ChannelControl   Channel = "control"
	ChannelTelemetry Channel = "telemetry"
)

// MessageType enumerates every message kind. Keeping them as typed constants
// avoids magic strings and makes the switch statements on both ends exhaustive.
type MessageType string

const (
	// --- Control channel: agent -> controller ---

	// TypeHello is the first frame an agent sends after the socket opens. It
	// authenticates the agent and advertises its capabilities.
	TypeHello MessageType = "hello"
	// TypePong answers a Ping from the controller.
	TypePong MessageType = "pong"
	// TypeAck acknowledges receipt of a telemetry batch up to a sequence number.
	TypeAck MessageType = "ack"
	// TypeTestState reports a transition in the local test lifecycle.
	TypeTestState MessageType = "test_state"

	// --- Control channel: controller -> agent ---

	// TypeWelcome accepts a Hello and returns server-side parameters (such as
	// the controller clock, used for offset estimation).
	TypeWelcome MessageType = "welcome"
	// TypeReject refuses a Hello (bad token, version mismatch, ...).
	TypeReject MessageType = "reject"
	// TypePing is a heartbeat probe.
	TypePing MessageType = "ping"
	// TypeAssignTest hands a fully-resolved, authorized test job to the agent.
	// The agent persists it locally so it can continue the test across a
	// controller disconnect.
	TypeAssignTest MessageType = "assign_test"
	// TypeStart tells the agent to begin a previously assigned test.
	TypeStart MessageType = "start"
	// TypeStop requests a graceful stop of the running test.
	TypeStop MessageType = "stop"
	// TypeAbort requests an immediate abort plus cleanup (kill processes,
	// revert any faults).
	TypeAbort MessageType = "abort"

	// --- Telemetry channel: agent -> controller ---

	// TypeMetricBatch carries a batch of time-stamped metric samples.
	TypeMetricBatch MessageType = "metric_batch"
	// TypeEvent carries a discrete event (error, kernel message, fault marker).
	TypeEvent MessageType = "event"
)

// Envelope is the single frame type exchanged over the socket. Payload is a raw
// message whose concrete type is determined by Type; decode it with the
// matching Decode* helper.
type Envelope struct {
	Channel Channel         `json:"ch"`
	Type    MessageType     `json:"type"`
	// TSWallMilli is the sender's wall-clock time (UTC, Unix milliseconds) at
	// the moment the envelope was produced.
	TSWallMilli int64           `json:"ts"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// --- Payload types ---

// Capabilities describes what a given host can actually do. The agent probes
// these at startup; the UI hides features a host cannot support rather than
// letting a user configure something that will fail.
type Capabilities struct {
	Fio          bool     `json:"fio"`
	FioVersion   string   `json:"fioVersion,omitempty"`
	IoUring      bool     `json:"ioUring"`
	Libaio       bool     `json:"libaio"`
	Multipath    bool     `json:"multipath"`
	DeviceMapper bool     `json:"deviceMapper"`
	Tc           bool     `json:"tc"`
	Smartctl     bool     `json:"smartctl"`
	Nvme         bool     `json:"nvme"`
	IoStat       bool     `json:"iostat"`
	// IoEngines lists fio I/O engines detected as available.
	IoEngines []string `json:"ioEngines,omitempty"`
}

// SystemInfo is a static snapshot of a host captured at registration and frozen
// with each test run. A result without reproducible conditions is worthless, so
// this travels with every run.
type SystemInfo struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Distro       string `json:"distro,omitempty"`
	KernelVers   string `json:"kernel,omitempty"`
	Arch         string `json:"arch"`
	CPUModel     string `json:"cpuModel,omitempty"`
	CPUCores     int    `json:"cpuCores,omitempty"`
	MemTotalMB   int64  `json:"memTotalMB,omitempty"`
	AgentVersion string `json:"agentVersion"`
}

// Hello authenticates an agent and advertises its identity and capabilities.
type Hello struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Token           string       `json:"token"`
	AgentID         string       `json:"agentId"`
	System          SystemInfo   `json:"system"`
	Capabilities    Capabilities `json:"capabilities"`
	// MonoNanos is the agent's monotonic clock reading at Hello time, used as
	// the zero reference for drift-free duration math on the controller.
	MonoNanos int64 `json:"monoNanos"`
}

// Welcome accepts a Hello.
type Welcome struct {
	// ControllerWallMilli lets the agent estimate the clock offset to the
	// controller in addition to NTP/chrony.
	ControllerWallMilli int64  `json:"controllerWallMilli"`
	HeartbeatSeconds    int    `json:"heartbeatSeconds"`
	SessionID           string `json:"sessionId"`
}

// Reject refuses a Hello and explains why.
type Reject struct {
	Reason string `json:"reason"`
}

// Ping is a heartbeat probe; Pong echoes the nonce back.
type Ping struct {
	Nonce int64 `json:"nonce"`
}

// Pong answers a Ping.
type Pong struct {
	Nonce int64 `json:"nonce"`
	// MonoNanos lets the controller track the agent's monotonic clock.
	MonoNanos int64 `json:"monoNanos"`
}

// Ack acknowledges telemetry up to and including Seq for a given run.
type Ack struct {
	RunID string `json:"runId"`
	Seq   uint64 `json:"seq"`
}

// TestStatePhase enumerates the local lifecycle of a test on an agent.
type TestStatePhase string

const (
	PhaseAssigned  TestStatePhase = "assigned"
	PhaseStarting  TestStatePhase = "starting"
	PhaseRunning   TestStatePhase = "running"
	PhaseStopping  TestStatePhase = "stopping"
	PhaseCompleted TestStatePhase = "completed"
	PhaseFailed    TestStatePhase = "failed"
	PhaseAborted   TestStatePhase = "aborted"
)

// TestState reports a lifecycle transition for a run on this agent.
type TestState struct {
	RunID   string         `json:"runId"`
	Phase   TestStatePhase `json:"phase"`
	Message string         `json:"message,omitempty"`
}

// AssignTest hands a resolved, authorized job to the agent. The concrete
// benchmark parameters live in Spec (kept as raw JSON here so the protocol
// package does not need to depend on the benchmark package; the agent decodes
// it with the benchmark schema).
type AssignTest struct {
	RunID string `json:"runId"`
	// AuthorizedUntilMilli bounds how long the agent may keep running this test
	// autonomously if the controller disconnects. A disconnect must not abort a
	// running test, but the authorization is still time-bounded for safety.
	AuthorizedUntilMilli int64           `json:"authorizedUntilMilli"`
	Spec                 json.RawMessage `json:"spec"`
}

// Start begins a previously assigned test.
type Start struct {
	RunID string `json:"runId"`
}

// Stop requests a graceful stop.
type Stop struct {
	RunID string `json:"runId"`
}

// Abort requests an immediate abort and cleanup.
type Abort struct {
	RunID  string `json:"runId"`
	Reason string `json:"reason,omitempty"`
}

// MetricSample is one time-stamped observation on a single host. All rate
// fields are instantaneous values for the sample interval unless noted.
type MetricSample struct {
	// TSWallMilli is the wall-clock time (UTC) for cross-node correlation.
	TSWallMilli int64 `json:"t"`
	// MonoNanos is the agent monotonic clock for drift-free durations.
	MonoNanos int64 `json:"m"`

	// Storage / fio-derived metrics.
	IOPSRead   float64 `json:"ior,omitempty"`
	IOPSWrite  float64 `json:"iow,omitempty"`
	BWReadKBs  float64 `json:"bwr,omitempty"`
	BWWriteKBs float64 `json:"bww,omitempty"`
	LatMeanUs  float64 `json:"lm,omitempty"`
	LatP95Us   float64 `json:"l95,omitempty"`
	LatP99Us   float64 `json:"l99,omitempty"`
	QueueDepth float64 `json:"qd,omitempty"`

	// Host metrics.
	CPUPercent  float64 `json:"cpu,omitempty"`
	MemPercent  float64 `json:"mem,omitempty"`
	LoadAvg1    float64 `json:"load,omitempty"`
	DiskUtilPct float64 `json:"du,omitempty"`
	NetRxKBs    float64 `json:"nrx,omitempty"`
	NetTxKBs    float64 `json:"ntx,omitempty"`
}

// MetricBatch is a sequence-numbered batch of samples for one run. Sequence
// numbers let the controller detect gaps and let the agent resend unacked
// batches after a reconnect, so no measurement data is lost on a temporary
// disconnect.
type MetricBatch struct {
	RunID   string         `json:"runId"`
	Seq     uint64         `json:"seq"`
	Samples []MetricSample `json:"samples"`
}

// EventSeverity classifies an event.
type EventSeverity string

const (
	SeverityInfo  EventSeverity = "info"
	SeverityWarn  EventSeverity = "warn"
	SeverityError EventSeverity = "error"
)

// Event is a discrete, time-stamped occurrence: a kernel I/O error, a fault
// injection marker, a path failover, etc. Events are correlated with metric
// samples on the shared timeline.
type Event struct {
	RunID       string        `json:"runId,omitempty"`
	TSWallMilli int64         `json:"t"`
	MonoNanos   int64         `json:"m"`
	Severity    EventSeverity `json:"sev"`
	Source      string        `json:"src"`
	Message     string        `json:"msg"`
}

// --- Encoding helpers ---

// NewEnvelope marshals a payload into an Envelope with the given channel/type.
func NewEnvelope(ch Channel, t MessageType, nowWallMilli int64, payload any) (Envelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Envelope{}, err
		}
		raw = b
	}
	return Envelope{Channel: ch, Type: t, TSWallMilli: nowWallMilli, Payload: raw}, nil
}

// Decode unmarshals the envelope payload into dst (a pointer).
func (e Envelope) Decode(dst any) error {
	if len(e.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(e.Payload, dst)
}
