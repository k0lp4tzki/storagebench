package agent

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/storagebench/storagebench/internal/fault"
)

// FaultController is the agent-side gatekeeper for network-fault injection. The
// agent itself never performs privileged actions; this controller validates and
// authorizes a request, then delegates execution to the privilege-broker.
//
// Safety gates enforced here (in addition to the broker's own re-validation):
//   - The management interface (carrying the controller connection) can never be
//     targeted, so a fault cannot cut the agent off from the controller.
//   - If a non-empty interface allowlist is configured, only those interfaces
//     are permitted.
//   - Active faults are tracked per interface so they can be reverted on abort,
//     disconnect, or shutdown.
type FaultController struct {
	log    *slog.Logger
	client *fault.Client

	mu        sync.Mutex
	mgmtIface string            // protected; set once the connection is up
	allowlist map[string]bool   // empty => all non-management interfaces allowed
	active    map[string]string // iface -> runID, for revert-on-cleanup
}

// NewFaultController creates a controller. socketPath empty disables faults.
func NewFaultController(log *slog.Logger, socketPath string, allowlist []string) *FaultController {
	var client *fault.Client
	if socketPath != "" {
		client = fault.NewClient(socketPath)
	}
	return &FaultController{
		log:       log,
		client:    client,
		allowlist: normalizeIfaceList(allowlist),
		active:    make(map[string]string),
	}
}

// Enabled reports whether fault injection is available (broker configured).
func (fc *FaultController) Enabled() bool {
	return fc.client != nil
}

// SetManagementIface records the protected interface. Called when a connection is
// established (and may change across reconnects if routing changes).
func (fc *FaultController) SetManagementIface(iface string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if iface != "" && iface != fc.mgmtIface {
		fc.log.Info("management interface detected (protected from faults)", "iface", iface)
	}
	fc.mgmtIface = iface
}

// authorize checks the interface against the safety gates. Caller holds no lock.
func (fc *FaultController) authorize(iface string) error {
	if err := fault.ValidateIface(iface); err != nil {
		return err
	}
	fc.mu.Lock()
	mgmt := fc.mgmtIface
	allow := fc.allowlist
	fc.mu.Unlock()

	if mgmt == "" {
		// We could not determine the management interface. Refuse rather than risk
		// cutting off the controller connection.
		return fmt.Errorf("management interface unknown; refusing fault for safety")
	}
	if iface == mgmt {
		return fmt.Errorf("interface %q carries the controller connection and is protected", iface)
	}
	if len(allow) > 0 && !allow[iface] {
		return fmt.Errorf("interface %q is not in the fault allowlist", iface)
	}
	return nil
}

// Apply authorizes and applies a fault via the broker, tracking it as active.
func (fc *FaultController) Apply(spec fault.Spec, ttlSeconds int, runID string) error {
	if fc.client == nil {
		return fmt.Errorf("fault injection is not enabled on this agent")
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	if err := fc.authorize(spec.Iface); err != nil {
		return err
	}
	if err := fc.client.Apply(spec, ttlSeconds, runID); err != nil {
		return err
	}
	fc.mu.Lock()
	fc.active[spec.Iface] = runID
	fc.mu.Unlock()
	fc.log.Info("fault applied", "iface", spec.Iface, "kind", spec.Kind, "run", runID)
	return nil
}

// Revert removes a fault on an interface via the broker.
func (fc *FaultController) Revert(iface, runID string) error {
	if fc.client == nil {
		return fmt.Errorf("fault injection is not enabled on this agent")
	}
	if err := fault.ValidateIface(iface); err != nil {
		return err
	}
	if err := fc.client.Revert(iface, runID); err != nil {
		return err
	}
	fc.mu.Lock()
	delete(fc.active, iface)
	fc.mu.Unlock()
	fc.log.Info("fault reverted", "iface", iface, "run", runID)
	return nil
}

// RevertAll reverts every fault this agent applied (best-effort). Used on abort,
// disconnect, or shutdown. It asks the broker to revert all tracked faults.
func (fc *FaultController) RevertAll(reason string) {
	if fc.client == nil {
		return
	}
	fc.mu.Lock()
	n := len(fc.active)
	fc.mu.Unlock()
	if n == 0 {
		return
	}
	reverted, err := fc.client.RevertAll()
	if err != nil {
		fc.log.Error("revert-all failed", "reason", reason, "err", err)
		return
	}
	fc.mu.Lock()
	fc.active = make(map[string]string)
	fc.mu.Unlock()
	fc.log.Warn("reverted all faults", "reason", reason, "ifaces", reverted)
}
