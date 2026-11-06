package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/storagebench/storagebench/internal/store"
)

// handleProfiles lists profiles (GET) or creates one (POST).
func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profs, err := s.store.ListProfiles()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list profiles failed")
			return
		}
		writeJSON(w, http.StatusOK, profs)
	case http.MethodPost:
		var p store.Profile
		if err := decodeJSON(w, r, &p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		created, err := s.store.CreateProfile(p)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

// handleProfileByID handles GET/PUT/DELETE /api/profiles/{id}.
func (s *Server) handleProfileByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/profiles/"), "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing profile id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		p, ok, err := s.store.GetProfile(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "get profile failed")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodPut:
		var p store.Profile
		if err := decodeJSON(w, r, &p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		p.ID = id
		updated, err := s.store.UpdateProfile(p)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.store.DeleteProfile(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
	default:
		methodNotAllowed(w, "GET, PUT, DELETE")
	}
}

// handleBatches lists batches (GET) or starts one (POST).
func (s *Server) handleBatches(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		batches, err := s.store.ListBatches(100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list batches failed")
			return
		}
		writeJSON(w, http.StatusOK, batches)
	case http.MethodPost:
		var req StartBatchRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		batchID, err := s.executor.Start(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"batchId": batchID})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

// handleBatchByID returns a batch and its runs: GET /api/batches/{id}.
func (s *Server) handleBatchByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/batches/"), "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing batch id")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	d, ok, err := s.store.GetBatch(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get batch failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "batch not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// decodeJSON decodes a JSON request body with a size limit and strict fields.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
