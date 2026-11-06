package controller

import (
	"net/http"
	"strings"
)

// handleRunDetail routes /api/runs/{id} and its sub-resources:
//
//	GET  /api/runs/{id}          -> run detail (config, nodes, summaries)
//	GET  /api/runs/{id}/metrics  -> time-series samples grouped by agent
//	GET  /api/runs/{id}/events   -> event log
//	POST /api/runs/{id}/stop     -> graceful stop
//	POST /api/runs/{id}/abort    -> forceful abort
func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		writeError(w, http.StatusBadRequest, "missing run id")
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	runID := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	switch sub {
	case "":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		s.runDetail(w, runID)
	case "metrics":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		s.runMetrics(w, runID)
	case "events":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		s.runEvents(w, runID)
	case "stop":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		s.runStop(w, runID, false)
	case "abort":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		s.runStop(w, runID, true)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) runDetail(w http.ResponseWriter, runID string) {
	d, ok, err := s.store.GetRun(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get run failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) runMetrics(w http.ResponseWriter, runID string) {
	m, err := s.store.GetRunMetrics(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get metrics failed")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) runEvents(w http.ResponseWriter, runID string) {
	ev, err := s.store.GetRunEvents(runID, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get events failed")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func (s *Server) runStop(w http.ResponseWriter, runID string, abort bool) {
	agents, err := s.store.GetRunAgents(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup agents failed")
		return
	}
	if len(agents) == 0 {
		writeError(w, http.StatusNotFound, "run not found or has no agents")
		return
	}
	if abort {
		s.hub.AbortRun(agents, runID, "aborted via UI")
	} else {
		s.hub.StopRun(agents, runID)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID, "action": actionWord(abort)})
}

func actionWord(abort bool) string {
	if abort {
		return "abort"
	}
	return "stop"
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
