// Command controller is the central coordinator and web server for the storage
// benchmark platform. It accepts agent connections, ingests telemetry, persists
// results, and serves the API/UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/storagebench/storagebench/internal/config"
	"github.com/storagebench/storagebench/internal/controller"
	"github.com/storagebench/storagebench/internal/logx"
	"github.com/storagebench/storagebench/internal/store"
	"github.com/storagebench/storagebench/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "controller: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "path to controller config JSON (optional; env overrides apply)")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("storagebench-controller " + version.String())
		return nil
	}

	cfg, err := config.LoadController(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logx.New(cfg.LogLevel)

	if cfg.AgentToken == "" {
		log.Warn("no agent token configured; set STORAGEBENCH_AGENT_TOKEN. Refusing insecure default is recommended for production.")
	}
	if !cfg.TLSEnabled() {
		log.Warn("TLS is not configured; controller is serving plaintext HTTP. Use TLS or a trusted tunnel in production.")
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer func() { _ = st.Close() }()
	log.Info("database ready", "path", cfg.DBPath)

	heartbeat := time.Duration(cfg.HeartbeatSeconds) * time.Second
	hub := controller.NewHub(heartbeat, st)
	srv := controller.NewServer(cfg, hub, st, log, nil)

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start the server.
	errc := make(chan error, 1)
	go func() {
		if cfg.TLSEnabled() {
			log.Info("controller listening (https)", "addr", cfg.ListenAddr)
			errc <- httpSrv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			log.Info("controller listening (http)", "addr", cfg.ListenAddr)
			errc <- httpSrv.ListenAndServe()
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Warn("graceful shutdown failed", "err", err)
	}
	log.Info("controller stopped")
	return nil
}
