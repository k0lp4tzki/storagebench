package agent

import (
	"io"
	"log/slog"
	"testing"

	"github.com/storagebench/storagebench/internal/fault"
)

func testFaultCtl(allow []string) *FaultController {
	return NewFaultController(slog.New(slog.NewTextHandler(io.Discard, nil)), "", allow)
}

func TestAuthorizeRefusesWhenMgmtUnknown(t *testing.T) {
	fc := testFaultCtl(nil)
	// mgmtIface not set -> must refuse for safety.
	if err := fc.authorize("eth0"); err == nil {
		t.Fatal("expected refusal when management interface is unknown")
	}
}

func TestAuthorizeProtectsManagementIface(t *testing.T) {
	fc := testFaultCtl(nil)
	fc.SetManagementIface("eth0")
	if err := fc.authorize("eth0"); err == nil {
		t.Fatal("expected management interface to be protected")
	}
	if err := fc.authorize("eth1"); err != nil {
		t.Fatalf("expected non-management interface to be allowed, got %v", err)
	}
}

func TestAuthorizeEnforcesAllowlist(t *testing.T) {
	fc := testFaultCtl([]string{"eth1"})
	fc.SetManagementIface("eth0")
	if err := fc.authorize("eth2"); err == nil {
		t.Fatal("expected interface outside allowlist to be refused")
	}
	if err := fc.authorize("eth1"); err != nil {
		t.Fatalf("expected allowlisted interface to pass, got %v", err)
	}
}

func TestAuthorizeRejectsInvalidIface(t *testing.T) {
	fc := testFaultCtl(nil)
	fc.SetManagementIface("eth0")
	if err := fc.authorize("eth0; rm -rf /"); err == nil {
		t.Fatal("expected invalid interface name to be rejected")
	}
}

func TestApplyDisabledWithoutBroker(t *testing.T) {
	fc := testFaultCtl(nil)
	fc.SetManagementIface("eth0")
	err := fc.Apply(fault.Spec{Kind: fault.KindLatency, Iface: "eth1", DelayMs: 10}, 60, "run1")
	if err == nil {
		t.Fatal("expected Apply to fail when broker is not configured")
	}
}
