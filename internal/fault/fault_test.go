package fault

import (
	"strings"
	"testing"
)

func TestValidateIfaceRejectsShellMeta(t *testing.T) {
	bad := []string{
		"", "eth0; rm -rf /", "eth 0", "eth0|x", "../dev", "eth0$(id)",
		"eth0`id`", "eth0\nx", "toolonginterfacename12345",
	}
	for _, b := range bad {
		if err := ValidateIface(b); err == nil {
			t.Errorf("expected iface %q to be rejected", b)
		}
	}
	good := []string{"eth0", "ens1f0", "bond0.100", "br-abc123", "enp3s0"}
	for _, g := range good {
		if err := ValidateIface(g); err != nil {
			t.Errorf("expected iface %q to be accepted, got %v", g, err)
		}
	}
}

func TestSpecValidateBounds(t *testing.T) {
	cases := []struct {
		name string
		s    Spec
		ok   bool
	}{
		{"latency ok", Spec{Kind: KindLatency, Iface: "eth0", DelayMs: 100}, true},
		{"latency jitter>delay", Spec{Kind: KindLatency, Iface: "eth0", DelayMs: 10, JitterMs: 20}, false},
		{"latency zero", Spec{Kind: KindLatency, Iface: "eth0", DelayMs: 0}, false},
		{"loss ok", Spec{Kind: KindLoss, Iface: "eth0", LossPercent: 12.5}, true},
		{"loss over 100", Spec{Kind: KindLoss, Iface: "eth0", LossPercent: 150}, false},
		{"loss zero", Spec{Kind: KindLoss, Iface: "eth0", LossPercent: 0}, false},
		{"bw ok", Spec{Kind: KindBandwidth, Iface: "eth0", RateKbit: 1000}, true},
		{"bw zero", Spec{Kind: KindBandwidth, Iface: "eth0", RateKbit: 0}, false},
		{"partition ok", Spec{Kind: KindPartition, Iface: "eth0"}, true},
		{"bad kind", Spec{Kind: "meltdown", Iface: "eth0"}, false},
	}
	for _, c := range cases {
		err := c.s.Validate()
		if c.ok && err != nil {
			t.Errorf("%s: expected valid, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: expected invalid", c.name)
		}
	}
}

func TestApplyCommandsNoShellAndExpectedArgs(t *testing.T) {
	s := Spec{Kind: KindLatency, Iface: "eth0", DelayMs: 100, JitterMs: 10}
	cmds, err := s.ApplyCommands()
	if err != nil {
		t.Fatalf("ApplyCommands: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	got := strings.Join(cmds[0].Args, " ")
	want := "tc qdisc add dev eth0 root netem delay 100ms 10ms"
	if got != want {
		t.Errorf("latency args = %q, want %q", got, want)
	}
	// Program must be allowlisted.
	if !AllowedPrograms[cmds[0].Args[0]] {
		t.Errorf("program %q not in allowlist", cmds[0].Args[0])
	}
}

func TestPartitionAndBandwidthArgs(t *testing.T) {
	p := Spec{Kind: KindPartition, Iface: "eth1"}
	cmds, _ := p.ApplyCommands()
	if strings.Join(cmds[0].Args, " ") != "tc qdisc add dev eth1 root netem loss 100%" {
		t.Errorf("partition args = %q", cmds[0].Args)
	}

	b := Spec{Kind: KindBandwidth, Iface: "eth1", RateKbit: 5000}
	cmds, _ = b.ApplyCommands()
	joined := strings.Join(cmds[0].Args, " ")
	if !strings.HasPrefix(joined, "tc qdisc add dev eth1 root tbf rate 5000kbit") {
		t.Errorf("bandwidth args = %q", joined)
	}
}

func TestRevertCommandsIdempotent(t *testing.T) {
	cmds, err := RevertCommands("eth0")
	if err != nil {
		t.Fatalf("RevertCommands: %v", err)
	}
	if len(cmds) != 1 || !cmds[0].IgnoreError {
		t.Fatalf("revert should be a single ignore-error command, got %+v", cmds)
	}
	if strings.Join(cmds[0].Args, " ") != "tc qdisc del dev eth0 root" {
		t.Errorf("revert args = %q", cmds[0].Args)
	}
	if err := func() error { _, e := RevertCommands("bad iface"); return e }(); err == nil {
		t.Error("expected revert to reject bad iface")
	}
}

func TestLossPercentFormatting(t *testing.T) {
	s := Spec{Kind: KindLoss, Iface: "eth0", LossPercent: 10}
	cmds, _ := s.ApplyCommands()
	if !strings.Contains(strings.Join(cmds[0].Args, " "), "loss 10%") {
		t.Errorf("whole-number loss should format as 10%%, got %q", cmds[0].Args)
	}
	s.LossPercent = 12.5
	cmds, _ = s.ApplyCommands()
	if !strings.Contains(strings.Join(cmds[0].Args, " "), "loss 12.5%") {
		t.Errorf("fractional loss should format as 12.5%%, got %q", cmds[0].Args)
	}
}
