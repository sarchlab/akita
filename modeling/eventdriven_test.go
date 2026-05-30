package modeling_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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
	lastTime  timing.VTimeInSec
}

func (p *mockProcessor) Process(
	comp *modeling.EventDrivenComponent[edSpec, edState],
	now timing.VTimeInSec,
) bool {
	p.callCount++
	p.lastTime = now

	s := &comp.State
	s.Count++
	s.Message = "processed"

	return true
}

// --- Builder tests ---

func TestEventDrivenBuilderBuild(t *testing.T) {
	engine := timing.NewSerialEngine()
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

	if comp.Spec != spec {
		t.Errorf("Spec() = %v, want %v", comp.Spec, spec)
	}

	if comp.State.Count != 0 {
		t.Errorf("State().Count = %d, want 0", comp.State.Count)
	}
}

// --- Spec, State, StateAssignment, StatePtr ---

func TestEventDrivenGetStateAssignment(t *testing.T) {
	engine := timing.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	comp.State = edState{Count: 42, Message: "hello"}

	got := comp.State
	if got.Count != 42 {
		t.Errorf("State().Count = %d, want 42", got.Count)
	}
	if got.Message != "hello" {
		t.Errorf("State().Message = %q, want %q", got.Message, "hello")
	}
}

func TestEventDrivenStatePtr(t *testing.T) {
	engine := timing.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	ptr := &comp.State
	ptr.Count = 99
	ptr.Message = "mutated"

	got := comp.State
	if got.Count != 99 {
		t.Errorf("State().Count = %d, want 99", got.Count)
	}
	if got.Message != "mutated" {
		t.Errorf("State().Message = %q, want %q", got.Message, "mutated")
	}
}

// --- Handle test ---

func TestEventDrivenHandle(t *testing.T) {
	engine := timing.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	evt := timing.NewEventBase(10, comp.Name())

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

	got := comp.State
	if got.Count != 1 {
		t.Errorf("State().Count = %d, want 1", got.Count)
	}
	if got.Message != "processed" {
		t.Errorf("State().Message = %q, want %q", got.Message, "processed")
	}
}

// --- NotifyRecv and NotifyPortFree ---

func TestEventDrivenNotifyRecv(t *testing.T) {
	engine := timing.NewSerialEngine()
	proc := &mockProcessor{}

	comp := modeling.NewEventDrivenBuilder[edSpec, edState]().
		WithEngine(engine).
		WithProcessor(proc).
		Build("EDComp")

	// NotifyRecv should not panic.
	comp.NotifyRecv(nil)
}

func TestEventDrivenNotifyPortFree(t *testing.T) {
	engine := timing.NewSerialEngine()
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
	engine := timing.NewSerialEngine()
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
