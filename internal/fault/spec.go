// Package fault defines the abstracted, validated description of a network fault
// and turns it into a safe, fixed set of `tc`/`iproute2` command argument
// vectors. It is the first line of defense for the resilience-testing phase:
//
//   - Users configure faults through high-level, allowlisted fields. They never
//     type tc/netem parameters or anything that becomes a shell command.
//   - Commands are produced as argument arrays ([]string), never as a shell
//     string. The privilege-broker executes them with exec (no `sh -c`), so
//     there is no shell to inject into.
//   - The interface name is validated against a strict character allowlist and,
//     on the agent, additionally checked against a runtime allowlist that always
//     excludes the management interface carrying the controller connection.
//
// Network faults are implemented with netem (and tbf for bandwidth limiting) on
// the egress qdisc of a chosen interface. They are non-destructive and fully
// reversible by deleting the qdisc, which the broker also does automatically on
// a watchdog timeout.
package fault

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind enumerates the supported network fault types.
type Kind string

const (
	// KindLatency adds a fixed delay (optionally with jitter) to egress packets.
	KindLatency Kind = "latency"
	// KindLoss drops a percentage of egress packets at random.
	KindLoss Kind = "loss"
	// KindBandwidth caps egress bandwidth (token bucket filter).
	KindBandwidth Kind = "bandwidth"
	// KindPartition drops effectively all egress traffic on the interface,
	// simulating a full network partition. Implemented as netem loss 100% so it
	// is reverted exactly like any other qdisc (unlike `ip link down`).
	KindPartition Kind = "partition"
)

var validKinds = map[Kind]bool{
	KindLatency: true, KindLoss: true, KindBandwidth: true, KindPartition: true,
}

// Spec is the abstracted description of one network fault on one interface.
type Spec struct {
	Kind Kind `json:"kind"`
	// Iface is the network interface the fault is applied to (egress qdisc).
	Iface string `json:"iface"`

	// DelayMs is the added latency in milliseconds (KindLatency).
	DelayMs int `json:"delayMs,omitempty"`
	// JitterMs is optional latency jitter in milliseconds (KindLatency).
	JitterMs int `json:"jitterMs,omitempty"`
	// LossPercent is the packet-loss percentage 0..100 (KindLoss).
	LossPercent float64 `json:"lossPercent,omitempty"`
	// RateKbit is the egress bandwidth cap in kilobit/s (KindBandwidth).
	RateKbit int `json:"rateKbit,omitempty"`
}

// Conservative upper bounds to prevent absurd configurations.
const (
	maxDelayMs  = 60_000      // 60s
	maxJitterMs = 10_000      // 10s
	maxRateKbit = 100_000_000 // 100 Gbit/s
)

// ifaceRe allows typical Linux interface names: letters, digits, and a few
// separators used by predictable names and VLAN/bridge devices (eth0, ens1f0,
// bond0.100, br-abc123). It deliberately excludes whitespace, slashes, and any
// shell metacharacter.
var ifaceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,14}$`)

// Validate checks the spec against the allowlists and bounds. A spec that passes
// is safe to turn into command argument vectors.
func (s *Spec) Validate() error {
	if !validKinds[s.Kind] {
		return fmt.Errorf("invalid fault kind %q", s.Kind)
	}
	if err := ValidateIface(s.Iface); err != nil {
		return err
	}
	switch s.Kind {
	case KindLatency:
		if s.DelayMs < 1 || s.DelayMs > maxDelayMs {
			return fmt.Errorf("delayMs must be between 1 and %d, got %d", maxDelayMs, s.DelayMs)
		}
		if s.JitterMs < 0 || s.JitterMs > maxJitterMs {
			return fmt.Errorf("jitterMs must be between 0 and %d, got %d", maxJitterMs, s.JitterMs)
		}
		if s.JitterMs > s.DelayMs {
			return fmt.Errorf("jitterMs (%d) must not exceed delayMs (%d)", s.JitterMs, s.DelayMs)
		}
	case KindLoss:
		if s.LossPercent <= 0 || s.LossPercent > 100 {
			return fmt.Errorf("lossPercent must be between 0 (exclusive) and 100, got %v", s.LossPercent)
		}
	case KindBandwidth:
		if s.RateKbit < 1 || s.RateKbit > maxRateKbit {
			return fmt.Errorf("rateKbit must be between 1 and %d, got %d", maxRateKbit, s.RateKbit)
		}
	case KindPartition:
		// No extra parameters; partition is loss=100%.
	}
	return nil
}

// ValidateIface checks an interface name against the strict character allowlist.
// It does not check existence or the runtime allowlist; the agent does that with
// host knowledge (and always excludes the management interface).
func ValidateIface(iface string) error {
	if strings.TrimSpace(iface) == "" {
		return fmt.Errorf("iface must not be empty")
	}
	if !ifaceRe.MatchString(iface) {
		return fmt.Errorf("iface %q has an invalid format", iface)
	}
	return nil
}

// Describe returns a short human-readable summary for logs and UI.
func (s *Spec) Describe() string {
	switch s.Kind {
	case KindLatency:
		if s.JitterMs > 0 {
			return fmt.Sprintf("Latenz +%dms ±%dms auf %s", s.DelayMs, s.JitterMs, s.Iface)
		}
		return fmt.Sprintf("Latenz +%dms auf %s", s.DelayMs, s.Iface)
	case KindLoss:
		return fmt.Sprintf("Paketverlust %.1f%% auf %s", s.LossPercent, s.Iface)
	case KindBandwidth:
		return fmt.Sprintf("Bandbreite %d kbit/s auf %s", s.RateKbit, s.Iface)
	case KindPartition:
		return fmt.Sprintf("Partition (100%% Verlust) auf %s", s.Iface)
	default:
		return string(s.Kind)
	}
}
