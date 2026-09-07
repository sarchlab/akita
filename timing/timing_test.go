package timing

import "testing"

type testEvent struct {
	EventBase
	label string
}

type recordingHandler struct {
	labels []string
}

func (h *recordingHandler) Handle(e Event) {
	h.labels = append(h.labels, e.(testEvent).label)
}

func TestSerialEngineRunsEventsInTimeOrder(t *testing.T) {
	engine := NewSerialEngine()
	var ids IDGenerator
	handler := &recordingHandler{}
	engine.RegisterHandler("handler", handler)

	engine.Schedule(testEvent{
		EventBase: MakeEventBase(ids.NewID(), 2, "handler"),
		label:     "second",
	})
	engine.Schedule(testEvent{
		EventBase: MakeEventBase(ids.NewID(), 1, "handler"),
		label:     "first",
	})

	if err := engine.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got, want := handler.labels, []string{"first", "second"}; !sameStrings(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestSerialEngineRunsSecondaryEventsAfterPrimaryEvents(t *testing.T) {
	engine := NewSerialEngine()
	var ids IDGenerator
	handler := &recordingHandler{}
	engine.RegisterHandler("handler", handler)

	secondary := testEvent{
		EventBase: MakeEventBase(ids.NewID(), 1, "handler"),
		label:     "secondary",
	}
	secondary.Secondary = true

	engine.Schedule(secondary)
	engine.Schedule(testEvent{
		EventBase: MakeEventBase(ids.NewID(), 1, "handler"),
		label:     "primary",
	})

	if err := engine.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got, want := handler.labels, []string{"primary", "secondary"}; !sameStrings(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestIDGeneratorSequence(t *testing.T) {
	var ids IDGenerator
	if got := ids.NewID(); got != 1 {
		t.Fatalf("first ID = %d, want 1", got)
	}
	if got := ids.NewID(); got != 2 {
		t.Fatalf("second ID = %d, want 2", got)
	}
}

func TestFreqNextTick(t *testing.T) {
	if got, want := GHz.NextTick(102_011), VTimeInPicoSec(103_000); got != want {
		t.Fatalf("GHz.NextTick(102011) = %d, want %d", got, want)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// Constructing an event preserves the supplied ID without requiring an engine
// or simulation, including when reconstructing an existing event.
func TestEventBaseUsesExplicitID(t *testing.T) {
	evt := MakeEventBase(42, 100, "handler")
	if evt.ID != 42 || evt.Time() != 100 || evt.HandlerID() != "handler" {
		t.Fatalf("event did not preserve its arguments: %+v", evt)
	}
}
