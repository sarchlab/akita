package modeling_test

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// --- Test types for EventDrivenComponent ---

type edSpec struct {
	Capacity int    `json:"capacity"`
	Label    string `json:"label"`
}

type edState struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}

// --- Mock processor ---

type mockProcessor struct {
	callCount int
	lastTime  sim.VTimeInSec
}

func (p *mockProcessor) Process(
	comp *modeling.EventDrivenComponent[edSpec, edState],
	now sim.VTimeInSec,
) bool {
	p.callCount++
	p.lastTime = now

	s := comp.GetStatePtr()
	s.Count++
	s.Message = "processed"

	return true
}

// --- Builder tests ---

func TestEventDrivenBuilderBuild(t *testing.T) {
	engine := sim.NewSerialEngine()
	spec := edSpec{Capacity: 10, Label: "test"}
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithSpec(spec).
		WithProcessor(proc).
		Build("EDComp")

	if comp.Name() != "EDComp" {
		t.Errorf("Name() = %q, want %q", comp.Name(), "EDComp")
	}

	if comp.GetSpec() != spec {
		t.Errorf("GetSpec() = %v, want %v", comp.GetSpec(), spec)
	}

	if comp.GetState().Count != 0 {
		t.Errorf("GetState().Count = %d, want 0", comp.GetState().Count)
	}
}

// --- GetSpec, GetState, SetState, GetStatePtr ---

func TestEventDrivenGetSetState(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	comp.SetState(edState{Count: 42, Message: "hello"})

	got := comp.GetState()
	if got.Count != 42 {
		t.Errorf("GetState().Count = %d, want 42", got.Count)
	}
	if got.Message != "hello" {
		t.Errorf("GetState().Message = %q, want %q", got.Message, "hello")
	}
}

func TestEventDrivenGetStatePtr(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	ptr := comp.GetStatePtr()
	ptr.Count = 99
	ptr.Message = "mutated"

	got := comp.GetState()
	if got.Count != 99 {
		t.Errorf("GetState().Count = %d, want 99", got.Count)
	}
	if got.Message != "mutated" {
		t.Errorf("GetState().Message = %q, want %q", got.Message, "mutated")
	}
}

// --- Handle test ---

func TestEventDrivenHandle(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	evt := sim.NewEventBase(10, comp)

	err := comp.Handle(evt)
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	if proc.callCount != 1 {
		t.Errorf("processor called %d times, want 1", proc.callCount)
	}
	if proc.lastTime != 10 {
		t.Errorf("processor lastTime = %v, want 10", proc.lastTime)
	}

	got := comp.GetState()
	if got.Count != 1 {
		t.Errorf("GetState().Count = %d, want 1", got.Count)
	}
	if got.Message != "processed" {
		t.Errorf("GetState().Message = %q, want %q", got.Message, "processed")
	}
}

// --- SaveState/LoadState ---

func TestEventDrivenSaveLoadState(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}
	spec := edSpec{Capacity: 5, Label: "save-test"}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithSpec(spec).
		WithProcessor(proc).
		Build("EDComp")

	comp.SetState(edState{Count: 77, Message: "saved"})

	var buf bytes.Buffer
	if err := comp.SaveState(&buf); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Create a new component and load state into it.
	comp2 := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp2")

	if err := comp2.LoadState(&buf); err != nil {
		t.Fatalf("LoadState() error: %v", err)
	}

	if comp2.GetSpec() != spec {
		t.Errorf("loaded spec = %v, want %v", comp2.GetSpec(), spec)
	}
	if comp2.GetState().Count != 77 {
		t.Errorf("loaded state count = %d, want 77", comp2.GetState().Count)
	}
	if comp2.GetState().Message != "saved" {
		t.Errorf("loaded state message = %q, want %q",
			comp2.GetState().Message, "saved")
	}
}

// --- ResetWakeup ---

func TestEventDrivenResetWakeup(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	// After build, pendingWakeup is -1, so scheduling should work.
	// Schedule a wakeup, then reset and schedule again — no panic.
	comp.ScheduleWakeAt(10)
	comp.ResetWakeup()
	comp.ScheduleWakeAt(5) // Should succeed since we reset.
}

// --- NotifyRecv and NotifyPortFree ---

func TestEventDrivenNotifyRecv(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	// NotifyRecv should not panic.
	comp.NotifyRecv(nil)
}

func TestEventDrivenNotifyPortFree(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	// NotifyPortFree should not panic.
	comp.NotifyPortFree(nil)
}

// --- Dedup guard test ---

func TestEventDrivenScheduleWakeAtDedup(t *testing.T) {
	engine := sim.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	// First schedule should work.
	comp.ScheduleWakeAt(50)

	// Scheduling at a later time should be a no-op (dedup).
	comp.ScheduleWakeAt(100)

	// Scheduling at an earlier time should replace.
	comp.ScheduleWakeAt(20)
}
