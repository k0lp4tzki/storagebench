package protocol

import (
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	batch := MetricBatch{
		RunID: "run-123",
		Seq:   7,
		Samples: []MetricSample{
			{TSWallMilli: 1000, IOPSRead: 12345.6, LatP99Us: 42.0},
			{TSWallMilli: 2000, IOPSWrite: 999.0},
		},
	}
	env, err := NewEnvelope(ChannelTelemetry, TypeMetricBatch, 1234567890, batch)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.Channel != ChannelTelemetry {
		t.Errorf("channel = %q, want %q", env.Channel, ChannelTelemetry)
	}
	if env.Type != TypeMetricBatch {
		t.Errorf("type = %q, want %q", env.Type, TypeMetricBatch)
	}

	var got MetricBatch
	if err := env.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.RunID != batch.RunID || got.Seq != batch.Seq {
		t.Errorf("header mismatch: got %+v", got)
	}
	if len(got.Samples) != 2 {
		t.Fatalf("samples len = %d, want 2", len(got.Samples))
	}
	if got.Samples[0].IOPSRead != 12345.6 {
		t.Errorf("IOPSRead = %v, want 12345.6", got.Samples[0].IOPSRead)
	}
	if got.Samples[1].IOPSWrite != 999.0 {
		t.Errorf("IOPSWrite = %v, want 999.0", got.Samples[1].IOPSWrite)
	}
}

func TestDecodeEmptyPayload(t *testing.T) {
	env := Envelope{Channel: ChannelControl, Type: TypePing}
	var p Ping
	if err := env.Decode(&p); err != nil {
		t.Fatalf("Decode empty: %v", err)
	}
	if p.Nonce != 0 {
		t.Errorf("Nonce = %d, want 0", p.Nonce)
	}
}
