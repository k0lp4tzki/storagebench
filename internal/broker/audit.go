package broker

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// AuditLog is an append-only, line-delimited JSON record of every privileged
// action the broker performs. It is intentionally simple and self-contained so
// the privileged process has no extra dependencies.
type AuditLog struct {
	mu sync.Mutex
	f  *os.File
}

// AuditEntry is one audit record.
type AuditEntry struct {
	Time    string `json:"time"`
	Action  string `json:"action"`
	RunID   string `json:"runId,omitempty"`
	Iface   string `json:"iface,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// OpenAuditLog opens (creating if needed) the audit log at path for appending.
func OpenAuditLog(path string) (*AuditLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	return &AuditLog{f: f}, nil
}

// Record appends one entry. Failures to write are swallowed (auditing must never
// break the control path), but are rare for a local append.
func (a *AuditLog) Record(action, runID, iface, detail string, success bool, errMsg string) {
	if a == nil || a.f == nil {
		return
	}
	entry := AuditEntry{
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		Action:  action,
		RunID:   runID,
		Iface:   iface,
		Detail:  detail,
		Success: success,
		Error:   errMsg,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = a.f.Write(append(b, '\n'))
}

// Close closes the audit log file.
func (a *AuditLog) Close() error {
	if a == nil || a.f == nil {
		return nil
	}
	return a.f.Close()
}
