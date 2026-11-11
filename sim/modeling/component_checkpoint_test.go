package modeling_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/queueing"
)

type ckptState struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

func TestCheckpointRoundTrip(t *testing.T) {
	src := ckptState{Count: 42, Names: []string{"a", "b"}}

	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, src, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	var dst ckptState
	if err := modeling.ReadCheckpoint(&buf, &dst, nil); err != nil {
		t.Fatalf("ReadCheckpoint: %v", err)
	}

	if dst.Count != 42 {
		t.Fatalf("State.Count = %d, want 42", dst.Count)
	}
	if strings.Join(dst.Names, ",") != "a,b" {
		t.Fatalf("State.Names = %v, want [a b]", dst.Names)
	}
}

type bufState struct {
	Items queueing.Buffer[int] `json:"items"`
}

// TestCheckpointPreservesStateBuffer proves that a queueing.Buffer held in a
// component's State round-trips through the checkpoint. Before queueing gained
// MarshalJSON/UnmarshalJSON this dropped the contents silently.
func TestCheckpointPreservesStateBuffer(t *testing.T) {
	src := bufState{Items: queueing.MakeBuffer[int](8)}
	src.Items.Push(7)
	src.Items.Push(8)

	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, src, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	dst := bufState{Items: queueing.MakeBuffer[int](8)}
	if err := modeling.ReadCheckpoint(&buf, &dst, nil); err != nil {
		t.Fatalf("ReadCheckpoint: %v", err)
	}

	if dst.Items.Size() != 2 {
		t.Fatalf("restored buffer size = %d, want 2", dst.Items.Size())
	}
	for _, want := range []int{7, 8} {
		if got, _ := dst.Items.Pop(); got != want {
			t.Fatalf("Pop = %d, want %d", got, want)
		}
	}
}
