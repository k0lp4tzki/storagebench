package controller

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/storagebench/storagebench/internal/fault"
	"github.com/storagebench/storagebench/internal/protocol"
)

// InjectFaultRequest is the body for POST /api/runs/{id}/faults.
type InjectFaultRequest struct {
	AgentID    string     `json:"agentId"`
	Spec       fault.Spec `json:"spec"`
	TTLSeconds int        `json:"ttlSeconds"`
}

// RevertFaultRequest is the body for POST /api/runs/{id}/faults/revert.
type RevertFaultRequest struct {
	AgentID string `json:"agentId"`
	Iface   string `json:"iface"`
}

// handleRunFaults handles fault endpoints under a run:
//
//	GET  /api/runs/{id}/faults          -> list fault actions (timeline)
//	POST /api/runs/{id}/faults          -> inject a fault
//	POST /api/runs/{id}/faults/revert   -> revert a fault
func (s *Server) handleRunFaults(w http.ResponseWriter, r *http.Request, runID, sub string) {
	switch {
	case sub == "faults" && r.Method == http.MethodGet:
		faults, err := s.store.ListRunFaults(runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list faults failed")
			return
		}
		writeJSON(w, http.StatusOK, faults)
	case sub == "faults" && r.Method == http.MethodPost:
		s.injectFault(w, r, runID)
	case sub == "faults/revert" && r.Method == http.MethodPost:
		s.revertFault(w, r, runID)
	default:
		http.NotFound(w, r)
	}
}

// injectFault validates the request, enforces safety gates, sends the inject to
// the target agent, and records it for the run timeline.
func (s *Server) injectFault(w http.ResponseWriter, r *http.Request, runID string) {
	var req InjectFaultRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "agentId is required")
		return
	}
	if err := req.Spec.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Safety gate: the target host must advertise the fault-injection capability.
	if !s.agentSupportsFaults(req.AgentID) {
		writeError(w, http.StatusBadRequest, "target host does not support fault injection (no reachable broker)")
		return
	}
	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = 3600
	}

	faultID := newFaultID()
	specRaw, err := json.Marshal(req.Spec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode spec failed")
		return
	}
	msg := protocol.InjectFault{RunID: runID, FaultID: faultID, Spec: specRaw, TTLSeconds: ttl}
	if err := s.hub.SendControl(req.AgentID, protocol.TypeInjectFault, msg); err != nil {
		writeError(w, http.StatusBadGateway, "could not reach agent: "+err.Error())
		return
	}

	// Record the inject on the run timeline. The agent also emits a correlated
	// event; this row drives the chart markers and revert pairing.
	if _, err := s.store.RecordFaultInject(runID, faultID, req.AgentID,
		string(req.Spec.Kind), req.Spec.Iface, req.Spec.Describe(), time.Now().UTC().UnixMilli()); err != nil {
		s.log.Warn("record fault inject failed", "run", runID, "err", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"faultId": faultID, "status": "injected"})
}

// revertFault sends a revert to the target agent and stamps the revert time.
func (s *Server) revertFault(w http.ResponseWriter, r *http.Request, runID string) {
	var req RevertFaultRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.AgentID) == "" || strings.TrimSpace(req.Iface) == "" {
		writeError(w, http.StatusBadRequest, "agentId and iface are required")
		return
	}
	if err := fault.ValidateIface(req.Iface); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := protocol.RevertFault{RunID: runID, Iface: req.Iface}
	if err := s.hub.SendControl(req.AgentID, protocol.TypeRevertFault, msg); err != nil {
		writeError(w, http.StatusBadGateway, "could not reach agent: "+err.Error())
		return
	}
	if err := s.store.RecordFaultRevert(runID, req.Iface, time.Now().UTC().UnixMilli()); err != nil {
		s.log.Warn("record fault revert failed", "run", runID, "err", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reverted"})
}

// agentSupportsFaults reports whether the given agent advertises fault injection.
func (s *Server) agentSupportsFaults(agentID string) bool {
	for _, a := range s.hub.Snapshot() {
		if a.AgentID == agentID {
			return a.Capabilities.FaultInjection && a.Connected
		}
	}
	return false
}

// newFaultID generates a short, unique fault id.
func newFaultID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("flt-%s-%s", time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(b[:]))
}
