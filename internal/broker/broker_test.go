package broker

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/storagebench/storagebench/internal/fault"
)

// newTestBroker returns a broker whose command execution is captured instead of
// shelling out to tc.
func newTestBroker() (*Broker, *recorder) {
	b := New(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	rec := &recorder{}
	b.execCmd = func(_ context.Context, name string, args []string) ([]byte, error) {
		rec.add(strings.Join(args, " "))
		if !fault.AllowedPrograms[name] {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, nil
	}
	return b, rec
}

type recorder struct {
	mu   sync.Mutex
	cmds []string
}

func (r *recorder) add(c string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, c)
}
func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.cmds))
	copy(out, r.cmds)
	return out
}

func TestBrokerApplyThenRevert(t *testing.T) {
	b, rec := newTestBroker()
	spec := fault.Spec{Kind: fault.KindLatency, Iface: "eth0", DelayMs: 50}
	if err := b.Apply(context.Background(), spec, 60, "run1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Apply first reverts any prior qdisc, then adds netem.
	cmds := rec.all()
	if len(cmds) < 2 {
		t.Fatalf("expected at least 2 commands, got %v", cmds)
	}
	if !strings.Contains(cmds[len(cmds)-1], "netem delay 50ms") {
		t.Errorf("last apply command unexpected: %v", cmds)
	}
	// Tracked active.
	b.mu.Lock()
	_, ok := b.active["eth0"]
	b.mu.Unlock()
	if !ok {
		t.Fatal("expected eth0 tracked as active after apply")
	}

	if err := b.Revert(context.Background(), "eth0", "run1"); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	b.mu.Lock()
	_, ok = b.active["eth0"]
	b.mu.Unlock()
	if ok {
		t.Fatal("expected eth0 no longer tracked after revert")
	}
}

func TestBrokerApplyRejectsInvalidSpec(t *testing.T) {
	b, _ := newTestBroker()
	bad := fault.Spec{Kind: fault.KindLatency, Iface: "eth0", DelayMs: 0}
	if err := b.Apply(context.Background(), bad, 60, "run1"); err == nil {
		t.Fatal("expected invalid spec to be rejected")
	}
	if err := b.Apply(context.Background(), fault.Spec{Kind: fault.KindLatency, Iface: "eth0", DelayMs: 10}, 0, "run1"); err == nil {
		t.Fatal("expected zero ttl to be rejected")
	}
}

func TestBrokerWatchdogAutoReverts(t *testing.T) {
	b, rec := newTestBroker()
	spec := fault.Spec{Kind: fault.KindPartition, Iface: "eth1"}
	// Apply with a very short TTL via direct arm: use Apply with ttl 1s then
	// shorten by re-arming through the public path is not available, so apply and
	// then manually fire the watchdog window using a 1s timer.
	if err := b.Apply(context.Background(), spec, 1, "run1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Wait for the watchdog to fire and revert.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		_, ok := b.active["eth1"]
		b.mu.Unlock()
		if !ok {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.mu.Lock()
	_, ok := b.active["eth1"]
	b.mu.Unlock()
	if ok {
		t.Fatal("expected watchdog to auto-revert eth1")
	}
	// A revert (del root) command must have been issued.
	found := false
	for _, c := range rec.all() {
		if strings.Contains(c, "qdisc del dev eth1 root") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a revert command for eth1, got %v", rec.all())
	}
}

func TestBrokerRevertAll(t *testing.T) {
	b, _ := newTestBroker()
	_ = b.Apply(context.Background(), fault.Spec{Kind: fault.KindLoss, Iface: "eth0", LossPercent: 10}, 60, "r")
	_ = b.Apply(context.Background(), fault.Spec{Kind: fault.KindLoss, Iface: "eth1", LossPercent: 10}, 60, "r")
	reverted := b.RevertAll(context.Background())
	if len(reverted) != 2 {
		t.Fatalf("expected 2 reverted, got %v", reverted)
	}
	b.mu.Lock()
	n := len(b.active)
	b.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected no active faults after RevertAll, got %d", n)
	}
}
