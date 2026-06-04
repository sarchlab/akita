package timing

import "testing"

type testEvent struct {
	*EventBase
	label string
}

type recordingHandler struct {
	labels []string
}

func (h *recordingHandler) Handle(e Event) error {
	h.labels = append(h.labels, e.(*testEvent).label)
	return nil
}

func TestSerialEngineRunsEventsInTimeOrder(t *testing.T) {
	ResetIDGenerator()

	engine := NewSerialEngine()
	handler := &recordingHandler{}
	engine.RegisterHandler("handler", handler)

	engine.Schedule(&testEvent{
		EventBase: NewEventBase(2, "handler"),
		label:     "second",
	})
	engine.Schedule(&testEvent{
		EventBase: NewEventBase(1, "handler"),
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
	ResetIDGenerator()

	engine := NewSerialEngine()
	handler := &recordingHandler{}
	engine.RegisterHandler("handler", handler)

	secondary := &testEvent{
		EventBase: NewEventBase(1, "handler"),
		label:     "secondary",
	}
	secondary.Secondary = true

	engine.Schedule(secondary)
	engine.Schedule(&testEvent{
		EventBase: NewEventBase(1, "handler"),
		label:     "primary",
	})

	if err := engine.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got, want := handler.labels, []string{"primary", "secondary"}; !sameStrings(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestIDGeneratorNextID(t *testing.T) {
	ResetIDGenerator()
	UseSequentialIDGenerator()

	GetIDGenerator().Generate()
	GetIDGenerator().Generate()

	if got, want := GetIDGeneratorNextID(), uint64(2); got != want {
		t.Fatalf("GetIDGeneratorNextID() = %d, want %d", got, want)
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
