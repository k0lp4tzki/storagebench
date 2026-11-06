package benchmark

import "testing"

// validSpec returns a minimal valid file-target spec for mutation in tests.
func validSpec() Spec {
	return Spec{
		Name:           "test",
		TargetKind:     TargetFile,
		TargetPath:     "/mnt/testvol/fio.dat",
		FileSizeMiB:    1024,
		Workload:       WorkloadRandRead,
		BlockSize:      "4k",
		IODepth:        32,
		NumJobs:        1,
		Engine:         EngineLibaio,
		DirectIO:       true,
		RuntimeSeconds: 60,
	}
}

func TestValidateAcceptsValidSpec(t *testing.T) {
	s := validSpec()
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid spec, got error: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Spec)
	}{
		{"empty name", func(s *Spec) { s.Name = "" }},
		{"bad workload", func(s *Spec) { s.Workload = "bogus" }},
		{"bad blocksize", func(s *Spec) { s.BlockSize = "3k" }},
		{"bad engine", func(s *Spec) { s.Engine = "magic" }},
		{"relative path", func(s *Spec) { s.TargetPath = "relative/path" }},
		{"path traversal", func(s *Spec) { s.TargetPath = "/mnt/../etc/passwd" }},
		{"newline in path", func(s *Spec) { s.TargetPath = "/mnt/x\nrw=write" }},
		{"zero iodepth", func(s *Spec) { s.IODepth = 0 }},
		{"too many jobs", func(s *Spec) { s.NumJobs = 99999 }},
		{"zero runtime", func(s *Spec) { s.RuntimeSeconds = 0 }},
		{"file size zero", func(s *Spec) { s.FileSizeMiB = 0 }},
		{"mixed bad ratio", func(s *Spec) { s.Workload = WorkloadRandRW; s.RWMixRead = 150 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSpec()
			tc.mutate(&s)
			if err := s.Validate(); err == nil {
				t.Errorf("expected validation error for %s, got nil", tc.name)
			}
		})
	}
}

func TestValidateDeviceWriteRequiresOptIn(t *testing.T) {
	s := validSpec()
	s.TargetKind = TargetDevice
	s.TargetPath = "/dev/sdx1"
	s.FileSizeMiB = 0
	s.Workload = WorkloadRandWrite

	if err := s.Validate(); err == nil {
		t.Fatal("expected write-on-device without allowDestructive to be rejected")
	}
	s.AllowDestructive = true
	if err := s.Validate(); err != nil {
		t.Fatalf("expected device write with allowDestructive to be valid, got: %v", err)
	}
}

func TestValidateDeviceMustBeUnderDev(t *testing.T) {
	s := validSpec()
	s.TargetKind = TargetDevice
	s.TargetPath = "/mnt/not-a-device"
	s.FileSizeMiB = 0
	if err := s.Validate(); err == nil {
		t.Fatal("expected device target outside /dev to be rejected")
	}
}
