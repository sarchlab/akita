package timing

import (
	"bytes"
	"reflect"
	"testing"
)

// queueTestEvent is a value-type event whose EventBase.ID identifies it.
type queueTestEvent struct {
	EventBase
}

type idRecordingHandler struct {
	ids []uint64
}

func (h *idRecordingHandler) Handle(e Event) error {
	h.ids = append(h.ids, e.(queueTestEvent).ID)
	return nil
}

func TestSerialEngineQueueRoundTrip(t *testing.T) {
	RegisterEvent(queueTestEvent{})

	mk := func(time VTimeInPicoSec, id uint64) queueTestEvent {
		e := queueTestEvent{}
		e.Time_ = time
		e.ID = id
		e.HandlerID_ = "h"
		return e
	}

	// Engine A schedules a mix of times, including two at the same time, then
	// is checkpointed without running.
	a := NewSerialEngine()
	a.Schedule(mk(5, 1))
	a.Schedule(mk(2, 2))
	a.Schedule(mk(5, 3))
	a.Schedule(mk(8, 4))

	var buf bytes.Buffer
	if err := a.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	// Engine B (freshly built, same handler) restores and runs.
	b := NewSerialEngine()
	rec := &idRecordingHandler{}
	b.RegisterHandler("h", rec)
	if err := b.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if err := b.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// (time, seq): 2 -> id 2, then the two at time 5 in schedule order (1, 3),
	// then 8 -> id 4.
	want := []uint64{2, 1, 3, 4}
	if !reflect.DeepEqual(rec.ids, want) {
		t.Fatalf("fire order = %v, want %v", rec.ids, want)
	}
	if b.CurrentTime() != 8 {
		t.Fatalf("current time = %d, want 8", b.CurrentTime())
	}
}

func TestSerialEngineLoadRejectsUnknownHandler(t *testing.T) {
	RegisterEvent(queueTestEvent{})

	a := NewSerialEngine()
	e := queueTestEvent{}
	e.Time_ = 1
	e.HandlerID_ = "missing"
	a.Schedule(e)

	var buf bytes.Buffer
	if err := a.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	b := NewSerialEngine() // no "missing" handler registered
	err := b.LoadCheckpoint(&buf)
	if err == nil {
		t.Fatalf("expected unknown-handler error")
	}
}
