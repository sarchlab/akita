package messaging

import (
	"bytes"
	"strings"
	"testing"
)

func newCkptPort(in, out int) *defaultPort {
	return NewPort(nil, in, out, "P").(*defaultPort)
}

func TestPortCheckpointRoundTrip(t *testing.T) {
	src := newCkptPort(4, 8)

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := newCkptPort(4, 8)
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
}

func TestPortCheckpointCapacityMismatch(t *testing.T) {
	src := newCkptPort(4, 8)

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := newCkptPort(16, 8) // incoming capacity differs
	err := dst.LoadCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "incoming capacity mismatch") {
		t.Fatalf("expected incoming capacity mismatch, got %v", err)
	}
}

func TestPortCheckpointRoundTripWithMessages(t *testing.T) {
	RegisterMsg(&registryTestMsg{})

	mk := func(id uint64, v int) *registryTestMsg {
		m := &registryTestMsg{Value: v}
		m.ID = id
		return m
	}

	src := newCkptPort(4, 4)
	src.incomingBuf.PushTyped(mk(1, 10))
	src.incomingBuf.PushTyped(mk(2, 20))
	src.outgoingBuf.PushTyped(mk(3, 30))

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := newCkptPort(4, 4)
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	in := dst.incomingBuf.Elements()
	if len(in) != 2 {
		t.Fatalf("incoming size = %d, want 2", len(in))
	}
	if m := in[0].(*registryTestMsg); m.Value != 10 || m.Meta().ID != 1 {
		t.Fatalf("incoming[0] = %+v", m)
	}
	if m := in[1].(*registryTestMsg); m.Value != 20 || m.Meta().ID != 2 {
		t.Fatalf("incoming[1] = %+v", m)
	}

	out := dst.outgoingBuf.Elements()
	if len(out) != 1 || out[0].(*registryTestMsg).Value != 30 {
		t.Fatalf("outgoing = %+v", out)
	}
}
