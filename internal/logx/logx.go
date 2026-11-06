// Package logx provides a tiny wrapper around the standard library slog so every
// binary logs structured, leveled output consistently. We intentionally avoid a
// third-party logging dependency to keep the footprint small.
package logx

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger writing structured text to stderr at the given
// level ("debug", "info", "warn", "error"). Unknown levels default to info.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}
