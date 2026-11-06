package fault

// Broker wire protocol. The agent (client) and the privilege-broker (server)
// exchange newline-delimited JSON requests/responses over a local Unix domain
// socket. The protocol is intentionally tiny and request/response only.
//
// Security properties enforced by the broker server:
//   - Only AllowedPrograms are ever executed, resolved to fixed absolute paths.
//   - Commands run via exec with an argv (never `sh -c`); there is no shell.
//   - Every request's spec is re-validated server-side; the client is not
//     trusted to have validated anything.
//   - Every applied fault has a watchdog: if no explicit revert and no keepalive
//     arrives within TTLSeconds, the broker reverts it automatically.
//   - Every action is appended to an audit log.

// BrokerAction is the operation a request asks the broker to perform.
type BrokerAction string

const (
	// ActionApply installs a fault and arms its watchdog.
	ActionApply BrokerAction = "apply"
	// ActionRevert removes a fault on an interface.
	ActionRevert BrokerAction = "revert"
	// ActionRevertAll removes all faults the broker currently tracks. Used on
	// agent shutdown / run end as a safety sweep.
	ActionRevertAll BrokerAction = "revert_all"
	// ActionPing checks the broker is alive and responding.
	ActionPing BrokerAction = "ping"
)

// BrokerRequest is one newline-delimited JSON request.
type BrokerRequest struct {
	Action BrokerAction `json:"action"`
	// Spec is the fault to apply (ActionApply) or identifies the interface to
	// revert (ActionRevert uses Spec.Iface).
	Spec Spec `json:"spec,omitempty"`
	// TTLSeconds bounds how long an applied fault may persist without an explicit
	// revert before the broker's watchdog removes it. Required for ActionApply.
	TTLSeconds int `json:"ttlSeconds,omitempty"`
	// RunID associates the action with a run for the audit log (optional).
	RunID string `json:"runId,omitempty"`
}

// BrokerResponse is the broker's reply.
type BrokerResponse struct {
	OK bool `json:"ok"`
	// Error is a human-readable message when OK is false.
	Error string `json:"error,omitempty"`
	// Reverted lists interfaces reverted by ActionRevertAll.
	Reverted []string `json:"reverted,omitempty"`
}
