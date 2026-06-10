package modeling_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type edckSpec struct {
	Latency int `json:"latency"`
}

type edckState struct {
	Count int `json:"count"`
}

type edckProc struct{}

func (edckProc) Process(
	_ *modeling.EventDrivenComponent[edckSpec, edckState, modeling.None],
	_ timing.VTimeInPicoSec,
) bool {
	return false
}

func buildEDckComp(latency int) *modeling.EventDrivenComponent[edckSpec, edckState, modeling.None] {
	return modeling.NewEventDrivenBuilder[edckSpec, edckState, modeling.None]().
		WithEngine(timing.NewSerialEngine()).
		WithSpec(edckSpec{Latency: latency}).
		WithProcessor(edckProc{}).
		Build("ED")
}

func TestEventDrivenCheckpointRoundTrip(t *testing.T) {
	src := buildEDckComp(7)
	src.State = edckState{Count: 99}

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := buildEDckComp(7)
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if dst.State.Count != 99 {
		t.Fatalf("State.Count = %d, want 99", dst.State.Count)
	}
}

func TestEventDrivenCheckpointSpecMismatch(t *testing.T) {
	src := buildEDckComp(7)

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := buildEDckComp(9) // different spec
	err := dst.LoadCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "spec hash mismatch") {
		t.Fatalf("expected spec hash mismatch, got %v", err)
	}
}
