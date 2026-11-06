// Package clock provides the time primitives used across the platform.
//
// Two clocks are used deliberately:
//   - Wall clock (UTC, Unix milliseconds) is used to correlate samples and
//     events across the three hosts. This relies on NTP/chrony being in sync,
//     which is a documented operating requirement.
//   - A monotonic nanosecond counter is used for drift-free durations (time to
//     failover, time to recovery, test runtime), which must not be affected by
//     wall-clock adjustments.
package clock

import "time"

// processStart anchors the monotonic counter to process start so the returned
// values are small and comparable within a single agent lifetime.
var processStart = time.Now()

// NowWallMilli returns the current wall-clock time in UTC as Unix milliseconds.
func NowWallMilli() int64 {
	return time.Now().UTC().UnixMilli()
}

// MonoNanos returns a monotonic nanosecond counter since process start. It is
// not affected by wall-clock changes and is suitable for measuring durations.
func MonoNanos() int64 {
	return int64(time.Since(processStart))
}
