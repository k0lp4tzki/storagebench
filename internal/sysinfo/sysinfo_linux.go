//go:build linux

package sysinfo

import (
	"os"
	"strconv"
	"strings"

	"github.com/storagebench/storagebench/internal/protocol"
)

// collectOS enriches the snapshot with Linux-specific details read from /proc
// and standard release files. All reads are best-effort.
func collectOS(si *protocol.SystemInfo) {
	si.Distro = readOSReleasePretty()
	si.KernelVers = readKernelVersion()
	si.CPUModel, si.CPUCores = readCPUInfo()
	si.MemTotalMB = readMemTotalMB()
}

// readOSReleasePretty extracts PRETTY_NAME from /etc/os-release (e.g. the SLES
// release string).
func readOSReleasePretty() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			v := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(v, "\"")
		}
	}
	return ""
}

// readKernelVersion returns the release string from /proc/sys/kernel/osrelease.
func readKernelVersion() string {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// readCPUInfo returns the model name and logical core count from /proc/cpuinfo.
func readCPUInfo() (string, int) {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", 0
	}
	var model string
	cores := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "processor") {
			cores++
		}
		if model == "" && strings.HasPrefix(line, "model name") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				model = strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return model, cores
}

// readMemTotalMB returns total memory in MB from /proc/meminfo.
func readMemTotalMB() int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

// refineCapabilities adds Linux-specific detail: fio version, available fio I/O
// engines, and whether io_uring/libaio are usable. Everything is best-effort.
func refineCapabilities(caps *protocol.Capabilities) {
	if caps.Fio {
		caps.FioVersion = parseFioVersion(trimOutput("fio", "--version"))
		engines := listFioEngines()
		caps.IoEngines = engines
		for _, e := range engines {
			switch e {
			case "io_uring":
				caps.IoUring = true
			case "libaio":
				caps.Libaio = true
			}
		}
	}
}

// parseFioVersion normalizes `fio --version` output (e.g. "fio-3.36").
func parseFioVersion(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimPrefix(s, "fio-")
}

// listFioEngines parses `fio --enghelp` to learn which I/O engines this fio
// build supports. Returns nil if it cannot be determined.
func listFioEngines() []string {
	out := trimOutput("fio", "--enghelp")
	if out == "" {
		return nil
	}
	var engines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// The header line is non-indented; engine names are indented tokens.
		if line == "" || strings.HasSuffix(line, ":") || strings.Contains(line, " ") {
			continue
		}
		engines = append(engines, line)
	}
	return engines
}
