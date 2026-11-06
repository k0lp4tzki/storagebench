//go:build linux

package agent

import (
	"os"
	"strconv"
	"strings"

	"github.com/storagebench/storagebench/internal/protocol"
)

// linuxCollector reads host metrics from /proc. CPU utilization is computed as a
// delta between consecutive samples, so the first sample reports 0% CPU.
type linuxCollector struct {
	prevIdle  uint64
	prevTotal uint64
	havePrev  bool
}

func newMetricsCollector() metricsCollector { return &linuxCollector{} }

func (c *linuxCollector) Collect() protocol.MetricSample {
	var s protocol.MetricSample
	s.CPUPercent = c.cpuPercent()
	s.MemPercent = memPercent()
	s.LoadAvg1 = loadAvg1()
	return s
}

// cpuPercent parses the aggregate line of /proc/stat and returns busy% since the
// previous call.
func (c *linuxCollector) cpuPercent() float64 {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "cpu ") {
			line = l
			break
		}
	}
	if line == "" {
		return 0
	}
	fields := strings.Fields(line)[1:] // drop "cpu"
	var total uint64
	var idle uint64
	for i, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		total += v
		// Fields: user nice system idle iowait irq softirq steal guest guest_nice
		// idle is index 3, iowait index 4; count both as "not busy".
		if i == 3 || i == 4 {
			idle += v
		}
	}
	defer func() {
		c.prevIdle = idle
		c.prevTotal = total
		c.havePrev = true
	}()
	if !c.havePrev {
		return 0
	}
	dTotal := float64(total - c.prevTotal)
	dIdle := float64(idle - c.prevIdle)
	if dTotal <= 0 {
		return 0
	}
	busy := (dTotal - dIdle) / dTotal * 100
	if busy < 0 {
		busy = 0
	}
	if busy > 100 {
		busy = 100
	}
	return busy
}

// memPercent returns used memory as a percentage using MemTotal and
// MemAvailable from /proc/meminfo.
func memPercent() float64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var total, avail float64
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			avail = v
		}
	}
	if total <= 0 {
		return 0
	}
	used := total - avail
	return used / total * 100
}

// loadAvg1 returns the 1-minute load average from /proc/loadavg.
func loadAvg1() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}
