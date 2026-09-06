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
	handler := &recordingHandler{}
	engine.RegisterHandler("handler", handler)

	engine.Schedule(testEvent{
		EventBase: MakeEventBase(engine, 2, "handler"),
		label:     "second",
	})
	engine.Schedule(testEvent{
		EventBase: MakeEventBase(engine, 1, "handler"),
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
	handler := &recordingHandler{}
	engine.RegisterHandler("handler", handler)

	secondary := testEvent{
		EventBase: MakeEventBase(engine, 1, "handler"),
		label:     "secondary",
	}
	secondary.Secondary = true

	engine.Schedule(secondary)
	engine.Schedule(testEvent{
		EventBase: MakeEventBase(engine, 1, "handler"),
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
	engine := NewSerialEngine()
	if got := engine.NewID(); got != 1 {
		t.Fatalf("first ID = %d, want 1", got)
	}
	if got := engine.NewID(); got != 2 {
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
