package ticking

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type testEngine struct {
	timing.Engine
	now       timing.VTimeInPicoSec
	scheduled []timing.Event
}

func (e *testEngine) CurrentTime() timing.VTimeInPicoSec {
	return e.now
}

func (e *testEngine) Schedule(event timing.Event) {
	e.scheduled = append(e.scheduled, event)
}

func newTestScheduler(secondary bool) (*Scheduler, *testEngine) {
	engine := &testEngine{now: timing.VTimeInPicoSec(10000)}
	sim := modeling.NewStandaloneSimulation(engine)

	if secondary {
		return NewSecondaryScheduler("TC", sim, 1*timing.GHz), engine
	}

	return NewScheduler("TC", sim, 1*timing.GHz), engine
}

func wantScheduled(t *testing.T, engine *testEngine, times ...timing.VTimeInPicoSec) {
	t.Helper()

	if len(engine.scheduled) != len(times) {
		t.Fatalf("scheduled %d ticks, want %d", len(engine.scheduled), len(times))
	}

	for i, e := range engine.scheduled {
		if e.Time() != times[i] || e.HandlerID() != "TC" {
			t.Errorf("tick %d at %d for %q, want %d for TC",
				i, e.Time(), e.HandlerID(), times[i])
		}
	}
}

func TestTickLaterSchedulesTheNextCycle(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickLater()

	wantScheduled(t, engine, 11000)
}

func TestTickNowSchedulesTheCurrentCycle(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickNow()

	wantScheduled(t, engine, 10000)
}

func TestSchedulerSchedulesOneTickPerCycle(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickLater()
	ts.TickLater()
	ts.TickNow() // a tick is already scheduled in the future

	wantScheduled(t, engine, 11000)
}

func TestSecondarySchedulerSchedulesSecondaryTicks(t *testing.T) {
	ts, engine := newTestScheduler(true)

	ts.TickLater()

	wantScheduled(t, engine, 11000)
	if !engine.scheduled[0].IsSecondary() {
		t.Errorf("the tick of a secondary scheduler is not secondary")
	}
}
