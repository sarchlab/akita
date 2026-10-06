package timing

import (
	"bytes"
	"sync/atomic"
	"testing"
)

func TestIDGeneratorCheckpointRoundTrip(t *testing.T) {
	src := &IDGenerator{}
	// Advance the counter a few times.
	for i := 0; i < 100; i++ {
		src.NewID()
	}

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := &IDGenerator{}
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if got := atomic.LoadUint64(&dst.nextID); got != 100 {
		t.Fatalf("restored nextID = %d, want 100", got)
	}
	// The next generated ID continues from the restored counter.
	if id := dst.NewID(); id != 101 {
		t.Fatalf("next ID = %d, want 101", id)
	}
}
