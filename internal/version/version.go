// Package version centralizes build/version metadata. Values are overridden at
// build time via -ldflags (see the Makefile). Keeping them here avoids
// scattering build stamps across binaries.
package version

var (
	// Version is the semantic version or git tag of the build.
	Version = "0.1.0-dev"
	// Commit is the short git commit hash, injected at build time.
	Commit = "unknown"
	// BuildDate is an RFC3339 timestamp, injected at build time.
	BuildDate = "unknown"
)

// String returns a compact human-readable version string.
func String() string {
	return Version + " (" + Commit + ", " + BuildDate + ")"
}
