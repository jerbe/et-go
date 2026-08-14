package lockstep

import "testing"

func TestFrameBufferBasics(t *testing.T) {
	fb := NewFrameBuffer()
	inputs := NewOneFrameInputs()
	inputs.Inputs[1] = &LSInput{Button: 1}
	fb.SetFrameInputs(1, inputs)
	if got, ok := fb.GetFrameInputs(1); !ok || got == nil || got.Inputs[1] == nil {
		t.Fatal("frame inputs missing")
	}
	fb.SetHash(1, 123)
	if hash, ok := fb.GetHash(1); !ok || hash != 123 {
		t.Fatal("hash mismatch")
	}
	fb.SetSnapshot(5, []byte("data"))
	if frame, data, ok := fb.GetNearestSnapshot(6); !ok || frame != 5 || string(data) != "data" {
		t.Fatal("snapshot mismatch")
	}
}

func TestFrameBufferZeroValueSupportsSnapshots(t *testing.T) {
	var fb FrameBuffer
	fb.SetSnapshot(1, []byte("snapshot"))
	frame, data, ok := fb.GetNearestSnapshot(1)
	if !ok || frame != 1 || string(data) != "snapshot" {
		t.Fatalf("zero-value snapshot = frame=%d data=%q ok=%v", frame, data, ok)
	}
}

func TestFrameBufferNilReceiverIsSafe(t *testing.T) {
	var fb *FrameBuffer
	if _, ok := fb.GetFrameInputs(1); ok {
		t.Fatal("nil FrameBuffer should not return frame inputs")
	}
	if _, ok := fb.GetHash(1); ok {
		t.Fatal("nil FrameBuffer should not return hash")
	}
	if _, _, ok := fb.GetNearestSnapshot(1); ok {
		t.Fatal("nil FrameBuffer should not return snapshot")
	}
	if fb.MaxFrame() != 0 {
		t.Fatal("nil FrameBuffer MaxFrame should be zero")
	}
}

func TestOneFrameInputsMatch(t *testing.T) {
	f := NewOneFrameInputs()
	if !f.IsEmpty() {
		t.Fatal("expected empty")
	}
	f.Inputs[1] = &LSInput{}
	if f.IsEmpty() {
		t.Fatal("should not be empty")
	}
}
