// Package config loads and validates runtime configuration for the controller
// and the agent. Configuration is JSON-on-disk with environment-variable
// overrides for secrets, so tokens need not be written into files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Controller is the controller's runtime configuration.
type Controller struct {
	// ListenAddr is the address the HTTP/WebSocket/UI server binds to,
	// e.g. "0.0.0.0:8443" or "127.0.0.1:8080".
	ListenAddr string `json:"listenAddr"`

	// AgentToken is the pre-shared token agents must present. Prefer providing
	// it via the STORAGEBENCH_AGENT_TOKEN environment variable rather than on
	// disk.
	AgentToken string `json:"agentToken"`

	// TLSCertFile and TLSKeyFile enable HTTPS/WSS. If both are empty the server
	// runs plaintext HTTP, which is only acceptable behind a trusted tunnel or
	// for local development.
	TLSCertFile string `json:"tlsCertFile"`
	TLSKeyFile  string `json:"tlsKeyFile"`

	// DBPath is the SQLite database file for results and history.
	DBPath string `json:"dbPath"`

	// HeartbeatSeconds controls how often the controller pings agents.
	HeartbeatSeconds int `json:"heartbeatSeconds"`

	// LogLevel is one of debug|info|warn|error.
	LogLevel string `json:"logLevel"`

	// LocalAgentURL, when set, is the WebSocket URL of the agent running on the
	// controller host itself (the "master is also a test node" requirement).
	// Informational for now; the controller connects to all agents uniformly.
	LocalAgentURL string `json:"localAgentUrl,omitempty"`
}

// DefaultController returns a Controller populated with safe defaults.
func DefaultController() Controller {
	return Controller{
		ListenAddr:       "0.0.0.0:8443",
		DBPath:           "./data/storagebench.db",
		HeartbeatSeconds: 5,
		LogLevel:         "info",
	}
}

// LoadController reads a controller config file (if path is non-empty), applies
// environment overrides, and validates the result.
func LoadController(path string) (Controller, error) {
	cfg := DefaultController()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read controller config: %w", err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("parse controller config: %w", err)
		}
	}
	// Environment overrides take precedence over file values for secrets.
	if v := os.Getenv("STORAGEBENCH_AGENT_TOKEN"); v != "" {
		cfg.AgentToken = v
	}
	if v := os.Getenv("STORAGEBENCH_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("STORAGEBENCH_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Controller) validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("listenAddr must be set")
	}
	if strings.TrimSpace(c.DBPath) == "" {
		return fmt.Errorf("dbPath must be set")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("tlsCertFile and tlsKeyFile must be set together")
	}
	if c.HeartbeatSeconds <= 0 {
		return fmt.Errorf("heartbeatSeconds must be positive")
	}
	return nil
}

// TLSEnabled reports whether the controller should serve over HTTPS/WSS.
func (c Controller) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}
