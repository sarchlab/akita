package modeling_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/internal/tickscheduler"

	"github.com/sarchlab/akita/v5/internal/testsim"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
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

// TestCheckpointRestoresPendingEventGuard shows that the scheduler's dedup guard
// round-trips: after restoring a pending tick, asking for the same tick again
// schedules nothing, since the engine's restored queue already holds it.
func TestCheckpointRestoresPendingEventGuard(t *testing.T) {
	newScheduler := func(handler timing.Handler) (*tickscheduler.Scheduler, *timing.SerialEngine) {
		engine := timing.NewSerialEngine()
		engine.RegisterHandler("C", handler)
		sim := testsim.New(engine, false)
		if err := sim.Initialize(); err != nil {
			t.Fatal(err)
		}

		return tickscheduler.NewScheduler("C", sim, 1*timing.GHz), engine
	}

	src, _ := newScheduler(handlerFunc(func(timing.Event) {}))
	src.TickLater()

	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, ckptState{}, src); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}
	if !strings.Contains(buf.String(), `"pending_event_guard":[1000]`) {
		t.Fatalf("checkpoint is missing the pending-event guard: %s", buf.String())
	}

	handled := 0
	dst, engine := newScheduler(handlerFunc(func(timing.Event) { handled++ }))
	var state ckptState
	if err := modeling.ReadCheckpoint(&buf, &state, dst); err != nil {
		t.Fatalf("ReadCheckpoint: %v", err)
	}

	dst.TickLater()

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
