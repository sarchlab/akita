package twowaybuffered

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

func newCkptPort(in, out int) *Port {
	return NewPort(in, out)
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
	mk := func(id uint64, v int) messaging.Msg {
		m := messaging.Msg{Payload: registryTestMsg{Value: v}}
		m.ID = id
		return m
	}

	src := newCkptPort(4, 4)
	src.incomingBuf.Push(bufferEntry{Msg: mk(1, 10), TaskID: 100 + 1})
	src.incomingBuf.Push(bufferEntry{Msg: mk(2, 20), TaskID: 100 + 2})
	src.outgoingBuf.Push(bufferEntry{Msg: mk(3, 30), TaskID: 100 + 3})

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
	if m := in[0].Msg; m.Payload.(registryTestMsg).Value != 10 || m.ID != 1 {
		t.Fatalf("incoming[0] = %+v", m)
	}
	if m := in[1].Msg; m.Payload.(registryTestMsg).Value != 20 || m.ID != 2 {
		t.Fatalf("incoming[1] = %+v", m)
	}

	if in[0].TaskID != 101 || in[1].TaskID != 102 || dst.PeekIncomingTaskID() != 101 {
		t.Fatalf("lost incoming task IDs: %+v", in)
	}
	out := dst.outgoingBuf.Elements()
	if len(out) != 1 || out[0].TaskID != 103 || out[0].Msg.Payload.(registryTestMsg).Value != 30 {
		t.Fatalf("outgoing = %+v", out)
	}
}

func TestPortRejectsLegacyCheckpoint(t *testing.T) {
	legacy := `{"incoming":{"capacity":4,"elements":[]},"outgoing":{"capacity":4,"elements":[]}}`
	err := newCkptPort(4, 4).LoadCheckpoint(strings.NewReader(legacy))
	if err == nil || !strings.Contains(err.Error(), "unsupported checkpoint version") {
		t.Fatalf("expected legacy checkpoint rejection, got %v", err)
	}
}
