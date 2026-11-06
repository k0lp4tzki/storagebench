//go:build !linux

package sysinfo

import "github.com/storagebench/storagebench/internal/protocol"

// collectOS is a no-op on non-Linux platforms. The project targets Linux at
// runtime; this stub exists only so the module compiles on a developer's
// non-Linux workstation for editing and local tooling.
func collectOS(_ *protocol.SystemInfo) {}

// refineCapabilities is a no-op on non-Linux platforms.
func refineCapabilities(_ *protocol.Capabilities) {}
