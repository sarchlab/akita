package messaging

import (
	"bytes"
	"strings"
	"testing"
)

type ckptTestMsg struct{ meta MsgMeta }

func (m *ckptTestMsg) Meta() *MsgMeta { return &m.meta }

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

func TestPortCheckpointRejectsNonEmpty(t *testing.T) {
	p := newCkptPort(4, 4)
	p.incomingBuf.PushTyped(&ckptTestMsg{})

	var buf bytes.Buffer
	err := p.SaveCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "non-empty buffers") {
		t.Fatalf("expected non-empty-buffer rejection, got %v", err)
	}
}
