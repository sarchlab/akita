package tickscheduler_test

import (
	"github.com/sarchlab/akita/v5/internal/tickscheduler"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"

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

func newTestScheduler(secondary bool) (*tickscheduler.Scheduler, *testEngine) {
	engine := &testEngine{now: timing.VTimeInPicoSec(10000)}
	sim := testsim.New(engine, false)

	if secondary {
		return tickscheduler.NewSecondaryScheduler("TC", sim, 1*timing.GHz), engine
	}

	return tickscheduler.NewScheduler("TC", sim, 1*timing.GHz), engine
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

func TestTickAtAlignsToTheFirstEdgeAtOrAfterTheTime(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickAt(12500) // between edges
	ts.TickAt(14000) // on an edge
	engine.now = 14500
	ts.TickAt(3000) // in the past: the current cycle

	wantScheduled(t, engine, 13000, 14000, 15000)
}

func TestTickAtDeduplicatesOnlyOnTheSameEdge(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickAt(15000)
	ts.TickAt(16000) // later than a pending tick: scheduled
	ts.TickAt(15000) // on a pending edge: dropped
	ts.TickAt(14500) // the same edge: dropped
	ts.TickAt(13000) // earlier: scheduled

	wantScheduled(t, engine, 15000, 16000, 13000)
}

func TestTickAtIsKeptWhileANextCycleTickIsPending(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickLater()
	ts.TickAt(20000)

	wantScheduled(t, engine, 11000, 20000)
}

func TestTickLaterAndTickNowAreNotHeldBackByAFutureTick(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickAt(20000)
	ts.TickNow()
	ts.TickLater()

	wantScheduled(t, engine, 20000, 10000, 11000)
}

// TestASupersededTickIsNotScheduledTwice covers a pending tick that an earlier
// one superseded: when the earlier tick asks for it again, the scheduler
// knows it is still pending.
func TestASupersededTickIsNotScheduledTwice(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickAt(15000)
	engine.now = 11000
	ts.TickLater()
	engine.now = 12000 // the tick at 12000 fires
	ts.TickAt(15000)

	wantScheduled(t, engine, 15000, 12000)
}

// TestATickAtTheCurrentTimeServesOnlyTheCurrentCycle covers requests made
// while the tick at the current time is handled: it satisfies a request for
// the current cycle, but not one for a later edge.
func TestATickAtTheCurrentTimeServesOnlyTheCurrentCycle(t *testing.T) {
	ts, engine := newTestScheduler(false)

	ts.TickLater()
	engine.now = 11000 // the tick at 11000 is handled
	ts.TickNow()
	ts.TickAt(11000)
	ts.TickAt(17000)

	wantScheduled(t, engine, 11000, 17000)
}

func TestSnapshotAndRestoreTheGuard(t *testing.T) {
	ts, _ := newTestScheduler(false)
	ts.TickAt(15000)
	ts.TickLater()

	saved := ts.Snapshot()
	if len(saved) != 2 || saved[0] != 11000 || saved[1] != 15000 {
		t.Fatalf("Snapshot = %v, want [11000 15000]", saved)
	}

	restored, engine := newTestScheduler(false)
	restored.Restore(saved)
	restored.TickLater()
	restored.TickAt(15000)

	wantScheduled(t, engine)
}

func TestSecondarySchedulerSchedulesSecondaryTicks(t *testing.T) {
	ts, engine := newTestScheduler(true)

	ts.TickLater()

	wantScheduled(t, engine, 11000)
	if !engine.scheduled[0].IsSecondary() {
		t.Errorf("the tick of a secondary scheduler is not secondary")
	}
}
