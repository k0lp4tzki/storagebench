package benchmark

import (
	"strings"
	"testing"
)

func TestBuildJobfileContainsExpectedOptions(t *testing.T) {
	s := validSpec()
	s.Workload = WorkloadRandRW
	s.RWMixRead = 70
	s.TargetIOPS = 5000

	rendered, err := BuildJobfile(&s, "/var/lib/storagebench/runs/run1")
	if err != nil {
		t.Fatalf("BuildJobfile: %v", err)
	}
	jf := rendered.JobfileContent

	mustContain := []string{
		"[global]",
		"ioengine=libaio",
		"rw=randrw",
		"bs=4k",
		"iodepth=32",
		"direct=1",
		"time_based=1",
		"runtime=60",
		"rwmixread=70",
		"rate_iops=5000",
		"clat_percentiles=1",
		"write_bw_log=",
		"[" + JobName + "]",
		"filename=/mnt/testvol/fio.dat",
		"size=1024M",
	}
	for _, m := range mustContain {
		if !strings.Contains(jf, m) {
			t.Errorf("jobfile missing %q.\n---\n%s", m, jf)
		}
	}
}

func TestBuildJobfileReadonlyGuardForReads(t *testing.T) {
	s := validSpec() // randread
	rendered, err := BuildJobfile(&s, "/tmp/run")
	if err != nil {
		t.Fatalf("BuildJobfile: %v", err)
	}
	if !strings.Contains(rendered.JobfileContent, "readonly=1") {
		t.Error("expected readonly=1 guard for a read-only workload")
	}
}

func TestBuildJobfileNoReadonlyForWrites(t *testing.T) {
	s := validSpec()
	s.Workload = WorkloadSeqWrite
	rendered, err := BuildJobfile(&s, "/tmp/run")
	if err != nil {
		t.Fatalf("BuildJobfile: %v", err)
	}
	if strings.Contains(rendered.JobfileContent, "readonly=1") {
		t.Error("did not expect readonly guard for a write workload")
	}
}

func TestBuildJobfileRejectsRelativeWorkDir(t *testing.T) {
	s := validSpec()
	if _, err := BuildJobfile(&s, "relative/dir"); err == nil {
		t.Error("expected error for relative workDir")
	}
}

func TestBuildJobfileRejectsInvalidSpec(t *testing.T) {
	s := validSpec()
	s.BlockSize = "bogus"
	if _, err := BuildJobfile(&s, "/tmp/run"); err == nil {
		t.Error("expected BuildJobfile to reject an invalid spec")
	}
}
