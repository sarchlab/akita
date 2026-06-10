package timing

import (
	"reflect"
	"testing"
)

func mkRunUntilEvent(time VTimeInPicoSec, id uint64) queueTestEvent {
	e := queueTestEvent{}
	e.Time_ = time
	e.ID = id
	e.HandlerID_ = "h"
	return e
}

func TestRunUntilStopsAtTimeBoundary(t *testing.T) {
	e := NewSerialEngine()
	rec := &idRecordingHandler{}
	e.RegisterHandler("h", rec)

	e.Schedule(mkRunUntilEvent(10, 1))
	e.Schedule(mkRunUntilEvent(20, 2))
	e.Schedule(mkRunUntilEvent(30, 3))
	e.Schedule(mkRunUntilEvent(40, 4))

	if err := e.RunUntil(25); err != nil {
		t.Fatalf("RunUntil: %v", err)
	}

	// Events at 10 and 20 fired; 30 and 40 remain queued.
	if !reflect.DeepEqual(rec.ids, []uint64{1, 2}) {
		t.Fatalf("fired = %v, want [1 2]", rec.ids)
	}
	if e.CurrentTime() != 20 {
		t.Fatalf("time = %d, want 20", e.CurrentTime())
	}
	if got := e.queue.Len(); got != 2 {
		t.Fatalf("remaining = %d, want 2", got)
	}

	// Continuing from the boundary drains the rest in order.
	if err := e.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reflect.DeepEqual(rec.ids, []uint64{1, 2, 3, 4}) {
		t.Fatalf("fired = %v, want [1 2 3 4]", rec.ids)
	}
	if e.CurrentTime() != 40 {
		t.Fatalf("time = %d, want 40", e.CurrentTime())
	}
}

// rescheduleHandler re-arms itself one step into the future until a cutoff,
// exercising events scheduled mid-run.
type rescheduleHandler struct {
	engine *SerialEngine
	fired  []VTimeInPicoSec
	step   VTimeInPicoSec
	until  VTimeInPicoSec
}

func (h *rescheduleHandler) Handle(e Event) error {
	now := e.Time()
	h.fired = append(h.fired, now)

	next := now + h.step
	if next <= h.until {
		ev := queueTestEvent{}
		ev.Time_ = next
		ev.HandlerID_ = "h"
		h.engine.Schedule(ev)
	}

	return nil
}

func TestRunUntilProcessesEventsScheduledWithinBoundary(t *testing.T) {
	e := NewSerialEngine()
	h := &rescheduleHandler{engine: e, step: 10, until: 100}
	e.RegisterHandler("h", h)

	e.Schedule(mkRunUntilEvent(10, 1))

	// The event at 10 reschedules 20, which reschedules 30, ... RunUntil(35)
	// must process the chain up to 30 and leave 40 queued.
	if err := e.RunUntil(35); err != nil {
		t.Fatalf("RunUntil: %v", err)
	}

	if !reflect.DeepEqual(h.fired, []VTimeInPicoSec{10, 20, 30}) {
		t.Fatalf("fired = %v, want [10 20 30]", h.fired)
	}
	if e.CurrentTime() != 30 {
		t.Fatalf("time = %d, want 30", e.CurrentTime())
	}
	if got := e.queue.Len(); got != 1 {
		t.Fatalf("remaining = %d, want 1 (the event at 40)", got)
	}
}
