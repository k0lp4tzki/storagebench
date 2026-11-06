package controller

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/storagebench/storagebench/internal/config"
	"github.com/storagebench/storagebench/internal/store"
	"github.com/storagebench/storagebench/internal/version"
)

// Server wires together the hub, store, HTTP API, and static UI into a single
// http.Handler. Keeping everything in one process (no separate UI server, no
// message broker) matches the "small, comprehensible stack" requirement.
type Server struct {
	cfg   config.Controller
	hub   *Hub
	store *store.Store
	log   *slog.Logger
	uiFS  fs.FS // optional embedded/served UI assets; may be nil
}

// NewServer constructs the controller server.
func NewServer(cfg config.Controller, hub *Hub, st *store.Store, log *slog.Logger, uiFS fs.FS) *Server {
	return &Server{cfg: cfg, hub: hub, store: st, log: log, uiFS: uiFS}
}

// Handler builds the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	heartbeat := time.Duration(s.cfg.HeartbeatSeconds) * time.Second

	// Agent WebSocket endpoint (control + telemetry).
	mux.Handle("/ws/agent", s.hub.AgentWSHandler(s.cfg.AgentToken, heartbeat, s.log))

	// REST API (read-only for the MVP slice; test creation lands next).
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/nodes", s.handleNodes)
	mux.HandleFunc("/api/runs", s.handleRuns)

	// Static UI, if provided. Falls back to a minimal placeholder page.
	if s.uiFS != nil {
		mux.Handle("/", http.FileServer(http.FS(s.uiFS)))
	} else {
		mux.HandleFunc("/", s.handlePlaceholder)
	}

	return s.withCommonHeaders(mux)
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

// handleRuns returns recent runs.
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list runs failed")
		return
	}
	writeJSON(w, http.StatusOK, runs)
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
