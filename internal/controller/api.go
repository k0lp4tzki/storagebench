package controller

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/storagebench/storagebench/internal/config"
	"github.com/storagebench/storagebench/internal/store"
	"github.com/storagebench/storagebench/internal/version"
)

// Server wires together the hub, store, HTTP API, and static UI into a single
// http.Handler. Keeping everything in one process (no separate UI server, no
// message broker) matches the "small, comprehensible stack" requirement.
type Server struct {
	cfg         config.Controller
	hub         *Hub
	store       *store.Store
	broadcaster *Broadcaster
	executor    *BatchExecutor
	log         *slog.Logger
	uiFS        fs.FS // optional embedded/served UI assets; may be nil
}

// NewServer constructs the controller server. broadcaster may be nil (the UI
// live stream is then unavailable but the rest of the API still works).
func NewServer(cfg config.Controller, hub *Hub, st *store.Store, broadcaster *Broadcaster, log *slog.Logger, uiFS fs.FS) *Server {
	return &Server{
		cfg:         cfg,
		hub:         hub,
		store:       st,
		broadcaster: broadcaster,
		executor:    NewBatchExecutor(hub, st, broadcaster, log),
		log:         log,
		uiFS:        uiFS,
	}
}

// Handler builds the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	heartbeat := time.Duration(s.cfg.HeartbeatSeconds) * time.Second

	// Agent WebSocket endpoint (control + telemetry).
	mux.Handle("/ws/agent", s.hub.AgentWSHandler(s.cfg.AgentToken, heartbeat, s.log))

	// Browser live-stream WebSocket (server -> browser push).
	mux.HandleFunc("/ws/ui", s.handleUIWebSocket)

	// REST API.
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/nodes", s.handleNodes)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail) // /api/runs/{id}[/metrics|/events|/stop|/abort]

	// Profiles (named collections of test steps) and batch execution.
	mux.HandleFunc("/api/profiles", s.handleProfiles)   // GET list, POST create
	mux.HandleFunc("/api/profiles/", s.handleProfileByID) // GET/PUT/DELETE /{id}
	mux.HandleFunc("/api/batches", s.handleBatches)     // GET list, POST start
	mux.HandleFunc("/api/batches/", s.handleBatchByID)  // GET /{id}

	// Static UI with SPA fallback, if an embedded/served bundle is provided.
	if s.uiFS != nil {
		mux.Handle("/", s.spaHandler())
	} else {
		mux.HandleFunc("/", s.handlePlaceholder)
	}

	return s.withCommonHeaders(mux)
}

// spaHandler serves the embedded single-page app. Requests for existing files
// are served directly; any other path falls back to index.html so client-side
// routing works on deep links / refresh.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never fall back for API or WS paths (handled by their own routes, but
		// guard here for safety).
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = "index.html"
		}
		if f, err := s.uiFS.Open(clean); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Fallback: serve index.html for SPA routes.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// withCommonHeaders adds baseline security headers to every response.
func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   version.Version,
		"commit":    version.Commit,
		"buildDate": version.BuildDate,
	})
}

// handleAgents returns the live view (connection status, RTT) from the hub.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.hub.Snapshot())
}

// handleNodes returns persisted node records (identity, capabilities).
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListNodes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list nodes failed")
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// handleRuns lists recent runs (GET) or starts a new run (POST).
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		runs, err := s.store.ListRuns(100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list runs failed")
			return
		}
		writeJSON(w, http.StatusOK, runs)
	case http.MethodPost:
		s.handleStartRun(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleStartRun decodes a StartRunRequest and launches it via the hub.
func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var req StartRunRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	runID, err := s.hub.StartRun(s.store, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
}

func (s *Server) handlePlaceholder(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(placeholderHTML))
}

// --- small JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

const placeholderHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>storagebench controller</title>
<style>
 body{font-family:system-ui,sans-serif;margin:3rem auto;max-width:48rem;padding:0 1rem;color:#1a1a1a}
 code{background:#f2f2f2;padding:.1rem .3rem;border-radius:4px}
 a{color:#2563eb}
</style></head>
<body>
<h1>storagebench controller</h1>
<p>The controller is running. The web UI bundle is not embedded in this build.</p>
<p>API endpoints:</p>
<ul>
 <li><a href="/api/health">/api/health</a></li>
 <li><a href="/api/version">/api/version</a></li>
 <li><a href="/api/agents">/api/agents</a> — live agent connections</li>
 <li><a href="/api/nodes">/api/nodes</a> — registered nodes</li>
 <li><a href="/api/runs">/api/runs</a> — recent runs</li>
</ul>
<p>Agents connect to <code>/ws/agent</code>.</p>
</body></html>`
