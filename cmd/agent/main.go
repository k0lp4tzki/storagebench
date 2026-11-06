// Command agent is the lightweight node daemon. It runs on every test host
// (including the controller host) and executes benchmarks, samples telemetry,
// and reports to the controller.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/storagebench/storagebench/internal/agent"
	"github.com/storagebench/storagebench/internal/config"
	"github.com/storagebench/storagebench/internal/logx"
	"github.com/storagebench/storagebench/internal/version"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "agent: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "path to agent config JSON (optional; env overrides apply)")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("storagebench-agent " + version.String())
		return nil
	}

	cfg, err := config.LoadAgent(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logx.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a := agent.New(cfg, log)
	return a.Run(ctx)
}
