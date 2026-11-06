package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Agent is the agent's runtime configuration.
type Agent struct {
	// ControllerURL is the WebSocket URL of the controller, e.g.
	// "wss://controller.example:8443/ws/agent" or "ws://127.0.0.1:8080/ws/agent".
	ControllerURL string `json:"controllerUrl"`

	// AgentID is a stable identifier for this agent/host. If empty, the agent
	// uses the system hostname.
	AgentID string `json:"agentId"`

	// Token is the pre-shared token presented to the controller. Prefer the
	// STORAGEBENCH_AGENT_TOKEN environment variable over writing it to disk.
	Token string `json:"token"`

	// InsecureSkipVerify disables TLS certificate verification. This is intended
	// only for self-signed certificates in isolated test environments and must
	// be used consciously; it is logged as a warning at startup.
	InsecureSkipVerify bool `json:"insecureSkipVerify"`

	// SpoolDir is where the agent persists authorized jobs and buffers telemetry
	// so a controller disconnect does not lose data or abort a running test.
	SpoolDir string `json:"spoolDir"`

	// SampleIntervalMS is the host-metric sampling interval in milliseconds.
	// Larger values reduce measurement overhead on the benchmark.
	SampleIntervalMS int `json:"sampleIntervalMs"`

	// ReconnectMinMS / ReconnectMaxMS bound the exponential backoff used when
	// reconnecting to the controller.
	ReconnectMinMS int `json:"reconnectMinMs"`
	ReconnectMaxMS int `json:"reconnectMaxMs"`

	// LogLevel is one of debug|info|warn|error.
	LogLevel string `json:"logLevel"`

	// --- Fault injection (resilience testing) ---

	// BrokerSocket is the Unix socket of the local privilege-broker. When empty,
	// fault injection is disabled on this agent (the agent never performs
	// privileged actions itself). Example: "/run/storagebench/broker.sock".
	BrokerSocket string `json:"brokerSocket"`

	// FaultIfaceAllowlist optionally restricts which interfaces may receive a
	// fault. When non-empty, only these interfaces are allowed. The management
	// interface carrying the controller connection is ALWAYS excluded regardless
	// of this list, so a fault can never cut the agent off from the controller.
	FaultIfaceAllowlist []string `json:"faultIfaceAllowlist"`
}

// DefaultAgent returns an Agent populated with safe defaults.
func DefaultAgent() Agent {
	return Agent{
		SpoolDir:         "./data/agent-spool",
		SampleIntervalMS: 1000,
		ReconnectMinMS:   1000,
		ReconnectMaxMS:   30000,
		LogLevel:         "info",
	}
}

// LoadAgent reads an agent config file (if path is non-empty), applies
// environment overrides, and validates the result.
func LoadAgent(path string) (Agent, error) {
	cfg := DefaultAgent()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read agent config: %w", err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("parse agent config: %w", err)
		}
	}
	if v := os.Getenv("STORAGEBENCH_AGENT_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("STORAGEBENCH_CONTROLLER_URL"); v != "" {
		cfg.ControllerURL = v
	}
	if v := os.Getenv("STORAGEBENCH_AGENT_ID"); v != "" {
		cfg.AgentID = v
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (a Agent) validate() error {
	if strings.TrimSpace(a.ControllerURL) == "" {
		return fmt.Errorf("controllerUrl must be set")
	}
	if !strings.HasPrefix(a.ControllerURL, "ws://") && !strings.HasPrefix(a.ControllerURL, "wss://") {
		return fmt.Errorf("controllerUrl must start with ws:// or wss://")
	}
	if a.SampleIntervalMS <= 0 {
		return fmt.Errorf("sampleIntervalMs must be positive")
	}
	if a.ReconnectMinMS <= 0 || a.ReconnectMaxMS < a.ReconnectMinMS {
		return fmt.Errorf("invalid reconnect backoff bounds")
	}
	if strings.TrimSpace(a.SpoolDir) == "" {
		return fmt.Errorf("spoolDir must be set")
	}
	return nil
}
