package controller

import (
	"net/http"
	"strings"
)

// handleCompare returns a cross-run comparison for the runs named in the
// "runs" query parameter (comma-separated run ids), preserving their order.
//
//	GET /api/compare?runs=id1,id2,...
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	ids := parseRunIDs(r.URL.Query().Get("runs"))
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "provide at least one run id via ?runs=")
		return
	}
	if len(ids) > 12 {
		writeError(w, http.StatusBadRequest, "too many runs to compare (max 12)")
		return
	}
	cmp, err := s.store.GetRunComparison(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "comparison failed")
		return
	}
	writeJSON(w, http.StatusOK, cmp)
}

// parseRunIDs splits a comma-separated list, trims, and drops empties while
// preserving order and removing duplicates.
func parseRunIDs(raw string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range strings.Split(raw, ",") {
		id := strings.TrimSpace(p)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
