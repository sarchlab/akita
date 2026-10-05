package modeling_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
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

func TestCheckpointRoundTrip(t *testing.T) {
	src := ckptState{Count: 42, Names: []string{"a", "b"}}

	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, ckptSpec{Latency: 7}, src, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	var dst ckptState
	if err := modeling.ReadCheckpoint(&buf, ckptSpec{Latency: 7}, &dst, nil); err != nil {
		t.Fatalf("ReadCheckpoint: %v", err)
	}

	if dst.Count != 42 {
		t.Fatalf("State.Count = %d, want 42", dst.Count)
	}
	if strings.Join(dst.Names, ",") != "a,b" {
		t.Fatalf("State.Names = %v, want [a b]", dst.Names)
	}
}

func TestCheckpointSpecMismatch(t *testing.T) {
	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, ckptSpec{Latency: 7}, ckptState{Count: 1}, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	var dst ckptState
	err := modeling.ReadCheckpoint(&buf, ckptSpec{Latency: 9}, &dst, nil)
	if err == nil || !strings.Contains(err.Error(), "spec hash mismatch") {
		t.Fatalf("expected spec hash mismatch, got %v", err)
	}
}

// TestCheckpointRestoresSchedulerGuard shows that the scheduler's dedup guard
// round-trips: after restoring a pending tick, asking for the same tick again
// schedules nothing, since the engine's restored queue already holds it.
func TestCheckpointRestoresSchedulerGuard(t *testing.T) {
	newScheduler := func() (*ticking.Scheduler, *timing.SerialEngine) {
		engine := timing.NewSerialEngine()
		engine.RegisterHandler("C", handlerFunc(func(timing.Event) {}))
		sim := modeling.NewStandaloneSimulation(engine)

		return ticking.NewScheduler("C", sim, 1*timing.GHz), engine
	}

	src, _ := newScheduler()
	src.TickLater()

	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, ckptSpec{}, ckptState{}, src); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	dst, engine := newScheduler()
	var state ckptState
	if err := modeling.ReadCheckpoint(&buf, ckptSpec{}, &state, dst); err != nil {
		t.Fatalf("ReadCheckpoint: %v", err)
	}

	dst.TickLater()

	handled := 0
	engine.RegisterHandler("C", handlerFunc(func(timing.Event) { handled++ }))
	if err := engine.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if handled != 0 {
		t.Fatalf("a restored guard should suppress the duplicate tick, "+
			"but %d ticks were scheduled", handled)
	}
}

type handlerFunc func(timing.Event)

func (f handlerFunc) Handle(e timing.Event) { f(e) }

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
	if err := modeling.WriteCheckpoint(&buf, ckptSpec{}, src, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	dst := bufState{Items: queueing.MakeBuffer[int](8)}
	if err := modeling.ReadCheckpoint(&buf, ckptSpec{}, &dst, nil); err != nil {
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
