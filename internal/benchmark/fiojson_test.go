package benchmark

import (
	"strings"
	"testing"
)

const sampleFioJSON = `{
  "fio version": "fio-3.36",
  "timestamp": 1730900000,
  "jobs": [
    {
      "jobname": "storagebench",
      "read": {
        "iops": 20000.5,
        "bw": 80002,
        "clat_ns": {
          "mean": 320000.0,
          "percentile": {
            "95.000000": 500000.0,
            "99.000000": 900000.0,
            "99.900000": 1500000.0
          }
        }
      },
      "write": {
        "iops": 0,
        "bw": 0,
        "clat_ns": { "mean": 0, "percentile": {} }
      }
    }
  ]
}`

func TestParseReportAndSummary(t *testing.T) {
	rep, err := ParseReport([]byte(sampleFioJSON))
	if err != nil {
		t.Fatalf("ParseReport: %v", err)
	}
	sum, ok := rep.Summary()
	if !ok {
		t.Fatal("expected a summary")
	}
	if sum.IOPSRead != 20000.5 {
		t.Errorf("IOPSRead = %v, want 20000.5", sum.IOPSRead)
	}
	if sum.BWReadKBs != 80002 {
		t.Errorf("BWReadKBs = %v, want 80002", sum.BWReadKBs)
	}
	// clat_ns mean 320000ns -> 320us.
	if sum.LatMeanUs != 320.0 {
		t.Errorf("LatMeanUs = %v, want 320", sum.LatMeanUs)
	}
	if sum.LatP95Us != 500.0 {
		t.Errorf("LatP95Us = %v, want 500", sum.LatP95Us)
	}
	if sum.LatP99Us != 900.0 {
		t.Errorf("LatP99Us = %v, want 900", sum.LatP99Us)
	}
	// clat_ns p99.9 1500000ns -> 1500us.
	if sum.LatP999Us != 1500.0 {
		t.Errorf("LatP999Us = %v, want 1500", sum.LatP999Us)
	}
}

func TestSummaryPrefersWriteWhenNoReads(t *testing.T) {
	j := `{"jobs":[{"jobname":"x",
		"read":{"iops":0,"bw":0,"clat_ns":{"mean":0,"percentile":{}}},
		"write":{"iops":1000,"bw":4000,"clat_ns":{"mean":250000,"percentile":{"99.000000":750000}}}}]}`
	rep, err := ParseReport([]byte(j))
	if err != nil {
		t.Fatalf("ParseReport: %v", err)
	}
	sum, _ := rep.Summary()
	if sum.LatMeanUs != 250.0 {
		t.Errorf("LatMeanUs = %v, want 250 (from write)", sum.LatMeanUs)
	}
	if sum.LatP99Us != 750.0 {
		t.Errorf("LatP99Us = %v, want 750 (from write)", sum.LatP99Us)
	}
}

func TestSplitConcatenatedJSON(t *testing.T) {
	// Two complete docs plus a trailing partial.
	buf := []byte(`{"a":1}` + "\n" + `{"b":2}` + "\n" + `{"c":`)
	docs, rem := SplitConcatenatedJSON(buf)
	if len(docs) != 2 {
		t.Fatalf("expected 2 complete docs, got %d", len(docs))
	}
	if !strings.Contains(string(docs[0]), `"a":1`) {
		t.Errorf("doc[0] = %s", docs[0])
	}
	if !strings.Contains(string(docs[1]), `"b":2`) {
		t.Errorf("doc[1] = %s", docs[1])
	}
	if !strings.HasPrefix(string(rem), `{"c":`) {
		t.Errorf("remainder = %q, want partial starting with {\"c\":", rem)
	}
}

func TestSplitConcatenatedJSONEmpty(t *testing.T) {
	docs, rem := SplitConcatenatedJSON([]byte("   \n  "))
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
	if len(rem) != 0 {
		t.Errorf("expected empty remainder, got %q", rem)
	}
}
