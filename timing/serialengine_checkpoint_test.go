package timing

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// queueTestEvent is a value-type event whose EventBase.ID identifies it.
type queueTestEvent struct {
	EventBase
}

type idRecordingHandler struct {
	ids []uint64
}

func (h *idRecordingHandler) Handle(e Event) {
	h.ids = append(h.ids, e.(queueTestEvent).ID)
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
	a.RegisterHandler("h", &idRecordingHandler{})
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
	a.RegisterHandler("missing", &idRecordingHandler{})
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

// TestScheduleRejectsUnregisteredHandler checks that both engines refuse an
// event for a handler nobody registered, at the Schedule call.
func TestScheduleRejectsUnregisteredHandler(t *testing.T) {
	for name, e := range map[string]Engine{
		"serial":   NewSerialEngine(),
		"parallel": NewParallelEngine(),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), `handler "nobody"`) {
					t.Fatalf("panic = %v, want one naming the handler", r)
				}
			}()

			e.Schedule(MakeEventBase(1, 1, "nobody"))
		})
	}
}
