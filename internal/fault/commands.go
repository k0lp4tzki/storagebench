package fault

import "fmt"

// Command is a single program invocation as an argument vector. The first entry
// is the program name; it is executed with exec (no shell). The broker resolves
// the program against a fixed absolute-path allowlist.
type Command struct {
	// Args is the full argv, e.g. ["tc", "qdisc", "add", ...]. Args[0] is the
	// program name (resolved to an allowlisted absolute path by the broker).
	Args []string `json:"args"`
	// IgnoreError marks a command whose failure is acceptable (used in revert,
	// where deleting a non-existent qdisc is fine).
	IgnoreError bool `json:"ignoreError,omitempty"`
}

// AllowedPrograms is the fixed set of program names the broker will execute for
// network faults. Anything else is refused. Kept here so the spec->command layer
// and the broker share one source of truth.
var AllowedPrograms = map[string]bool{
	"tc": true,
}

// ApplyCommands returns the argument vectors that install the fault on the
// interface's egress. The spec must already be valid (call Validate first).
//
// All faults are expressed as a single root qdisc on the interface:
//   - latency/loss/partition: a netem root qdisc.
//   - bandwidth: a tbf (token bucket filter) root qdisc.
//
// Only one fault is active per interface at a time (one root qdisc). The caller
// (agent) enforces that by tracking active faults per interface.
func (s *Spec) ApplyCommands() ([]Command, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	switch s.Kind {
	case KindLatency:
		args := []string{"tc", "qdisc", "add", "dev", s.Iface, "root", "netem",
			"delay", fmt.Sprintf("%dms", s.DelayMs)}
		if s.JitterMs > 0 {
			args = append(args, fmt.Sprintf("%dms", s.JitterMs))
		}
		return []Command{{Args: args}}, nil
	case KindLoss:
		args := []string{"tc", "qdisc", "add", "dev", s.Iface, "root", "netem",
			"loss", fmt.Sprintf("%s%%", trimFloat(s.LossPercent))}
		return []Command{{Args: args}}, nil
	case KindPartition:
		args := []string{"tc", "qdisc", "add", "dev", s.Iface, "root", "netem",
			"loss", "100%"}
		return []Command{{Args: args}}, nil
	case KindBandwidth:
		// tbf needs a burst and latency bound in addition to the rate. Choose a
		// burst proportional to the rate (a common rule of thumb) and a fixed
		// queueing latency bound.
		args := []string{"tc", "qdisc", "add", "dev", s.Iface, "root", "tbf",
			"rate", fmt.Sprintf("%dkbit", s.RateKbit),
			"burst", "32kbit",
			"latency", "400ms"}
		return []Command{{Args: args}}, nil
	default:
		return nil, fmt.Errorf("unsupported kind %q", s.Kind)
	}
}

// RevertCommands returns the argument vectors that remove the fault. Deleting the
// root qdisc reverts any of the fault kinds. Deleting a non-existent qdisc is
// treated as success (IgnoreError), so revert is idempotent.
func RevertCommands(iface string) ([]Command, error) {
	if err := ValidateIface(iface); err != nil {
		return nil, err
	}
	return []Command{
		{Args: []string{"tc", "qdisc", "del", "dev", iface, "root"}, IgnoreError: true},
	}, nil
}

// trimFloat formats a loss percentage without a trailing ".0" for whole numbers,
// keeping at most one decimal place (validation bounds it to 0..100).
func trimFloat(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}
