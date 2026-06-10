package modeling_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
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

type bufSpec struct {
	N int `json:"n"`
}

type bufState struct {
	Items queueing.Buffer[int] `json:"items"`
}

func buildBufComp() *modeling.Component[bufSpec, bufState, modeling.None] {
	c := modeling.NewBuilder[bufSpec, bufState, modeling.None]().
		WithEngine(timing.NewSerialEngine()).
		WithFreq(1 * timing.GHz).
		WithSpec(bufSpec{N: 1}).
		Build("C")
	c.State.Items = queueing.NewBuffer[int]("items", 8)
	return c
}

// TestComponentCheckpointPreservesStateBuffer proves that a queueing.Buffer held
// in a component's State round-trips through the component serializer. Before
// queueing gained MarshalJSON/UnmarshalJSON this dropped the contents silently.
func TestComponentCheckpointPreservesStateBuffer(t *testing.T) {
	src := buildBufComp()
	src.State.Items.PushTyped(7)
	src.State.Items.PushTyped(8)

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := buildBufComp()
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if dst.State.Items.Size() != 2 {
		t.Fatalf("restored buffer size = %d, want 2", dst.State.Items.Size())
	}
	for _, want := range []int{7, 8} {
		if got := dst.State.Items.Pop(); got != want {
			t.Fatalf("Pop = %d, want %d", got, want)
		}
	}
}
