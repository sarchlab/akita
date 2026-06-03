package timing

import (
	"bytes"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSequentialIDGeneratorCheckpointRoundTrip(t *testing.T) {
	src := &sequentialIDGenerator{}
	// Advance the counter a few times.
	for i := 0; i < 100; i++ {
		src.Generate()
	}

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := &sequentialIDGenerator{}
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if got := atomic.LoadUint64(&dst.nextID); got != 100 {
		t.Fatalf("restored nextID = %d, want 100", got)
	}
	// The next generated ID continues from the restored counter.
	if id := dst.Generate(); id != 101 {
		t.Fatalf("next ID = %d, want 101", id)
	}
}

func TestParallelIDGeneratorNotCheckpointable(t *testing.T) {
	g := &parallelIDGenerator{}
	if err := g.SaveCheckpoint(io.Discard); err == nil ||
		!strings.Contains(err.Error(), "not checkpointable") {
		t.Fatalf("expected not-checkpointable error, got %v", err)
	}
	if err := g.LoadCheckpoint(strings.NewReader("{}")); err == nil {
		t.Fatalf("expected not-checkpointable error on load")
	}
}
