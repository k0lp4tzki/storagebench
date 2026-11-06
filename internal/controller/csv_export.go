package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

// runExportCSV streams a run's results as CSV: a summary block (one row per host)
// followed by a time-series block (one row per sample). A single file keeps the
// export self-contained and easy to open in a spreadsheet.
//
//	GET /api/runs/{id}/export.csv
func (s *Server) runExportCSV(w http.ResponseWriter, runID string) {
	d, ok, err := s.store.GetRun(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get run failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	cmp, err := s.store.GetRunComparison([]string{runID})
	if err != nil || len(cmp) == 0 {
		writeError(w, http.StatusInternalServerError, "summary failed")
		return
	}
	metrics, _ := s.store.GetRunMetrics(runID)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "run-"+safeFile(runID)+".csv"))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"# run", d.ID, d.Name, d.Status, d.StartedAt, d.EndedAt})
	_ = cw.Write(nil)

	// Summary block.
	_ = cw.Write([]string{"section", "summary"})
	_ = cw.Write([]string{
		"host", "phase", "iops_read", "iops_write", "bw_read_kbs", "bw_write_kbs",
		"lat_mean_us", "lat_p95_us", "lat_p99_us", "lat_p999_us", "iops_cv",
	})
	for _, n := range cmp[0].Nodes {
		_ = cw.Write([]string{
			n.Hostname, n.Phase,
			f(n.Summary.IOPSRead), f(n.Summary.IOPSWrite),
			f(n.Summary.BWReadKBs), f(n.Summary.BWWriteKBs),
			f(n.Summary.LatMeanUs), f(n.Summary.LatP95Us),
			f(n.Summary.LatP99Us), f(n.Summary.LatP999Us),
			cvStr(n.IOPSCv),
		})
	}
	_ = cw.Write(nil)

	// Time-series block (sorted by host, then wall time).
	_ = cw.Write([]string{"section", "timeseries"})
	_ = cw.Write([]string{
		"host", "ts_wall_ms", "iops_read", "iops_write", "bw_read_kbs", "bw_write_kbs",
		"lat_mean_us", "lat_p95_us", "lat_p99_us", "queue_depth", "cpu_pct", "disk_util_pct",
	})
	hosts := make([]string, 0, len(metrics))
	for h := range metrics {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	hostName := map[string]string{}
	for _, n := range d.Nodes {
		hostName[n.AgentID] = n.Hostname
	}
	for _, agentID := range hosts {
		name := hostName[agentID]
		if name == "" {
			name = agentID
		}
		for _, m := range metrics[agentID] {
			_ = cw.Write([]string{
				name, strconv.FormatInt(m.TSWallMilli, 10),
				f(m.IOPSRead), f(m.IOPSWrite), f(m.BWReadKBs), f(m.BWWriteKBs),
				f(m.LatMeanUs), f(m.LatP95Us), f(m.LatP99Us), f(m.QueueDepth),
				f(m.CPUPercent), f(m.DiskUtilPct),
			})
		}
	}
}

// compareExportCSV streams a cross-run comparison as CSV: one row per
// (run, host) plus a per-run aggregate row.
//
//	GET /api/compare/export.csv?runs=id1,id2,...
func (s *Server) compareExportCSV(w http.ResponseWriter, r *http.Request) {
	ids := parseRunIDs(r.URL.Query().Get("runs"))
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "provide at least one run id via ?runs=")
		return
	}
	cmp, err := s.store.GetRunComparison(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "comparison failed")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "compare.csv"))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{
		"run_id", "run_name", "status", "scope", "host",
		"iops_read", "iops_write", "bw_read_kbs", "bw_write_kbs",
		"lat_mean_us", "lat_p95_us", "lat_p99_us", "lat_p999_us", "iops_cv",
	})
	for _, cr := range cmp {
		for _, n := range cr.Nodes {
			_ = cw.Write([]string{
				cr.ID, cr.Name, cr.Status, "host", n.Hostname,
				f(n.Summary.IOPSRead), f(n.Summary.IOPSWrite),
				f(n.Summary.BWReadKBs), f(n.Summary.BWWriteKBs),
				f(n.Summary.LatMeanUs), f(n.Summary.LatP95Us),
				f(n.Summary.LatP99Us), f(n.Summary.LatP999Us), cvStr(n.IOPSCv),
			})
		}
		a := cr.Aggregate
		_ = cw.Write([]string{
			cr.ID, cr.Name, cr.Status, "aggregate", "(alle Hosts)",
			f(a.IOPSRead), f(a.IOPSWrite), f(a.BWReadKBs), f(a.BWWriteKBs),
			f(a.LatMeanUs), f(a.LatP95Us), f(a.LatP99Us), f(a.LatP999Us), "",
		})
	}
}

func f(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func cvStr(cv float64) string {
	if cv < 0 {
		return ""
	}
	return strconv.FormatFloat(cv, 'f', 4, 64)
}

// safeFile keeps a run id safe for a Content-Disposition filename.
func safeFile(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "export"
	}
	return string(out)
}
