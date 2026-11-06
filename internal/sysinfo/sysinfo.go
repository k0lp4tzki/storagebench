// Package sysinfo collects a static snapshot of the host and probes which
// benchmark/diagnostic tools are available. The agent captures this at startup
// and the controller freezes it with every run so results are reproducible.
//
// Capability probing is deliberately conservative: a tool counts as available
// only if its binary is found on PATH. The UI uses these flags to hide features
// a host cannot support instead of letting a user configure something that will
// fail at runtime.
package sysinfo

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/storagebench/storagebench/internal/protocol"
	"github.com/storagebench/storagebench/internal/version"
)

// Collect returns a SystemInfo snapshot for the current host. Fields that
// cannot be determined are left empty rather than guessed.
func Collect(hostname string) protocol.SystemInfo {
	si := protocol.SystemInfo{
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: version.Version,
	}
	// Platform-specific enrichment (kernel, distro, cpu, memory) is provided by
	// collectOS, implemented per-GOOS. It must never panic; on unsupported
	// platforms it is a no-op.
	collectOS(&si)
	return si
}

// DetectCapabilities probes for the tools the platform can drive. This runs the
// generic PATH-based checks and then lets the platform layer refine results
// (for example, parsing `fio --version` or listing fio I/O engines).
func DetectCapabilities() protocol.Capabilities {
	caps := protocol.Capabilities{
		Fio:          hasBinary("fio"),
		Multipath:    hasBinary("multipath"),
		DeviceMapper: hasBinary("dmsetup"),
		Tc:           hasBinary("tc"),
		Smartctl:     hasBinary("smartctl"),
		Nvme:         hasBinary("nvme"),
		IoStat:       hasBinary("iostat"),
	}
	refineCapabilities(&caps)
	return caps
}

// hasBinary reports whether an executable is resolvable on PATH.
func hasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// trimOutput runs a command and returns trimmed stdout, or "" on any error.
// It is used for cheap version/identity probes only.
func trimOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
