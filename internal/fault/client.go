package fault

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Client is the agent-side client for the privilege-broker. It opens a short
// connection per request over the local Unix socket. The agent never performs
// privileged actions itself; it only asks the broker.
type Client struct {
	socketPath string
	timeout    time.Duration
	mu         sync.Mutex
}

// NewClient creates a broker client for the given Unix socket path.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath, timeout: 20 * time.Second}
}

// do sends one request and returns the response.
func (c *Client) do(req BrokerRequest) (BrokerResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return BrokerResponse{}, fmt.Errorf("connect broker: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	line, err := json.Marshal(req)
	if err != nil {
		return BrokerResponse{}, err
	}
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return BrokerResponse{}, fmt.Errorf("write broker request: %w", err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return BrokerResponse{}, fmt.Errorf("read broker response: %w", err)
		}
		return BrokerResponse{}, fmt.Errorf("broker closed connection without a response")
	}
	var resp BrokerResponse
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return BrokerResponse{}, fmt.Errorf("decode broker response: %w", err)
	}
	return resp, nil
}

// Ping checks that the broker is reachable and responding.
func (c *Client) Ping() error {
	resp, err := c.do(BrokerRequest{Action: ActionPing})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("broker ping failed: %s", resp.Error)
	}
	return nil
}

// Apply asks the broker to install a fault with the given TTL (seconds).
func (c *Client) Apply(spec Spec, ttlSeconds int, runID string) error {
	resp, err := c.do(BrokerRequest{Action: ActionApply, Spec: spec, TTLSeconds: ttlSeconds, RunID: runID})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("apply failed: %s", resp.Error)
	}
	return nil
}

// Revert asks the broker to remove the fault on an interface.
func (c *Client) Revert(iface, runID string) error {
	resp, err := c.do(BrokerRequest{Action: ActionRevert, Spec: Spec{Iface: iface}, RunID: runID})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("revert failed: %s", resp.Error)
	}
	return nil
}

// RevertAll asks the broker to remove all tracked faults.
func (c *Client) RevertAll() ([]string, error) {
	resp, err := c.do(BrokerRequest{Action: ActionRevertAll})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("revert_all failed: %s", resp.Error)
	}
	return resp.Reverted, nil
}
