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

// The read-only guard is fio's --readonly CLI flag, not a jobfile keyword.
// "readonly" must never appear in the jobfile (fio rejects it there).
func TestBuildJobfileNeverContainsReadonlyKeyword(t *testing.T) {
	for _, w := range []Workload{WorkloadRandRead, WorkloadSeqRead, WorkloadSeqWrite, WorkloadRandRW} {
		s := validSpec()
		s.Workload = w
		if w.isMixed() {
			s.RWMixRead = 50
		}
		rendered, err := BuildJobfile(&s, "/tmp/run")
		if err != nil {
			t.Fatalf("BuildJobfile(%s): %v", w, err)
		}
		if strings.Contains(rendered.JobfileContent, "readonly") {
			t.Errorf("jobfile for %s must not contain the invalid 'readonly' keyword:\n%s", w, rendered.JobfileContent)
		}
	}
}

func TestSpecIsReadOnly(t *testing.T) {
	cases := map[Workload]bool{
		WorkloadRandRead:  true,
		WorkloadSeqRead:   true,
		WorkloadRandWrite: false,
		WorkloadSeqWrite:  false,
		WorkloadRandRW:    false,
		WorkloadSeqRW:     false,
	}
	for w, want := range cases {
		s := validSpec()
		s.Workload = w
		if got := s.IsReadOnly(); got != want {
			t.Errorf("IsReadOnly(%s) = %v, want %v", w, got, want)
		}
	}
}

// The --readonly guard must only apply to read-only workloads on device targets,
// never on file targets (fio must be able to create its own test file).
func TestSpecWantsReadonlyGuard(t *testing.T) {
	// File + read: no guard (fio must create the file).
	sf := validSpec() // file target, randread
	if sf.WantsReadonlyGuard() {
		t.Error("file target read workload should NOT request the --readonly guard")
	}

	// Device + read: guard on.
	sd := validSpec()
	sd.TargetKind = TargetDevice
	sd.TargetPath = "/dev/sdx"
	sd.FileSizeMiB = 0
	if !sd.WantsReadonlyGuard() {
		t.Error("device target read workload SHOULD request the --readonly guard")
	}

	// Device + write: no guard.
	sdw := sd
	sdw.Workload = WorkloadSeqWrite
	sdw.AllowDestructive = true
	if sdw.WantsReadonlyGuard() {
		t.Error("device target write workload should NOT request the --readonly guard")
	}
}

// File targets must allow fio to create the test file.
func TestBuildJobfileFileTargetAllowsCreate(t *testing.T) {
	s := validSpec() // file target
	rendered, err := BuildJobfile(&s, "/tmp/run")
	if err != nil {
		t.Fatalf("BuildJobfile: %v", err)
	}
	if !strings.Contains(rendered.JobfileContent, "allow_file_create=1") {
		t.Errorf("file target jobfile should allow file creation:\n%s", rendered.JobfileContent)
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
