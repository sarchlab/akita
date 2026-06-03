package modeling_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type ckptSpec struct {
	Latency int `json:"latency"`
}

type ckptState struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

func buildCkptComp(latency int) *modeling.Component[ckptSpec, ckptState, modeling.None] {
	return modeling.NewBuilder[ckptSpec, ckptState, modeling.None]().
		WithEngine(timing.NewSerialEngine()).
		WithFreq(1 * timing.GHz).
		WithSpec(ckptSpec{Latency: latency}).
		Build("Comp")
}

func TestComponentCheckpointRoundTrip(t *testing.T) {
	src := buildCkptComp(7)
	src.State = ckptState{Count: 42, Names: []string{"a", "b"}}

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := buildCkptComp(7)
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if dst.State.Count != 42 {
		t.Fatalf("State.Count = %d, want 42", dst.State.Count)
	}
	if strings.Join(dst.State.Names, ",") != "a,b" {
		t.Fatalf("State.Names = %v, want [a b]", dst.State.Names)
	}
}

func TestComponentCheckpointSpecMismatch(t *testing.T) {
	src := buildCkptComp(7)
	src.State = ckptState{Count: 1}

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := buildCkptComp(9) // different spec
	err := dst.LoadCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "spec hash mismatch") {
		t.Fatalf("expected spec hash mismatch, got %v", err)
	}
}
