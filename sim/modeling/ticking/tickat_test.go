package ticking_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// --- A ticking component with timed work ---

type timerSpec struct {
	Freq timing.Freq `json:"freq"`
}

// timerState holds the work due at later times, as TickAt asks: Due is sorted
// by time, and AddOnTick is work a tick adds, standing for work that arrives
// while the component is ticking.
type timerState struct {
	Due       []timing.VTimeInPicoSec `json:"due"`
	AddOnTick []timing.VTimeInPicoSec `json:"add_on_tick"`
	Ticks     []timing.VTimeInPicoSec `json:"ticks"`
	Done      []timing.VTimeInPicoSec `json:"done"`
}

type timerPorts struct{}

type timerMWs struct {
	Timer *timerMW
}

type timerComp = ticking.Component[
	timerSpec, timerState, modeling.None, timerPorts, timerMWs]

// timerMW handles the work that is due, adds the work of AddOnTick, and asks
// for a tick at the earliest work left. It never makes progress, so only the
// ticks the component asks for fire.
type timerMW struct {
	comp *timerComp
}

func (m *timerMW) Handle(e timing.Event) bool {
	s := &m.comp.State
	now := e.Time()
	s.Ticks = append(s.Ticks, now)

	for len(s.Due) > 0 && s.Due[0] <= now {
		s.Done = append(s.Done, s.Due[0])
		s.Due = s.Due[1:]
	}

	if len(s.AddOnTick) > 0 {
		s.Due = append(s.Due, s.AddOnTick...)
		slices.Sort(s.Due)
		s.AddOnTick = nil
	}

	if len(s.Due) > 0 {
		m.comp.TickAt(s.Due[0])
	}

	return false
}

func (m *timerMWs) Initialize(c *timerComp) {
	*m = timerMWs{Timer: &timerMW{comp: c}}
}

func timerDefaultSpec() timerSpec {
	return timerSpec{Freq: 1 * timing.GHz}
}

func buildTimer(sim *akitasim.Simulation, name string) *timerComp {
	c := modeling.MakeBuilder[*timerComp](timerDefaultSpec()).
		WithSimulation(sim).
		Build(name)

	return c
}

// addWork adds work due at t and asks for a tick at it, as a component does
// when work arrives.
func addWork(c *timerComp, t timing.VTimeInPicoSec) {
	c.State.Due = append(c.State.Due, t)
	slices.Sort(c.State.Due)
	c.TickAt(t)
}

// callAt runs f in an event at time t, standing for whatever wakes the
// component from outside, such as a message on a port.
type callAt struct {
	timing.EventBase
	f func()
}

type callHandler struct{}

func (callHandler) Handle(e timing.Event) { e.(callAt).f() }

func scheduleCall(sim *akitasim.Simulation, handler string, t timing.VTimeInPicoSec, f func()) {
	sim.Engine().Schedule(callAt{
		EventBase: timing.MakeEventBase(sim.NewID(), t, handler),
		f:         f,
	})
}

func newTimerSim(t *testing.T, sim *akitasim.Simulation) *timerComp {
	t.Helper()

	c := buildTimer(sim, "T")
	sim.Engine().RegisterHandler("Poke", callHandler{})
	if err := sim.Initialize(); err != nil {
		t.Fatal(err)
	}

	return c
}

func wantTicks(t *testing.T, c *timerComp, ticks, done []timing.VTimeInPicoSec) {
	t.Helper()

	if !reflect.DeepEqual(c.State.Ticks, ticks) {
		t.Errorf("ticked at %v, want %v", c.State.Ticks, ticks)
	}
	if !reflect.DeepEqual(c.State.Done, done) {
		t.Errorf("work done at %v, want %v", c.State.Done, done)
	}
}

// --- Tests ---

func TestTickAtTicksAtTheFirstEdgeAtOrAfterTheTime(t *testing.T) {
	sim := newSim()
	c := newTimerSim(t, sim)

	addWork(c, 4500) // between edges: ticks at 5000

	run(t, sim)

	wantTicks(t, c, []timing.VTimeInPicoSec{5000}, []timing.VTimeInPicoSec{4500})
}

// TestTickLaterWhileATickAtIsPending shows that an earlier wakeup is not held
// back by a pending far-future tick, and that the far-future tick still fires.
func TestTickLaterWhileATickAtIsPending(t *testing.T) {
	sim := newSim()
	c := newTimerSim(t, sim)

	addWork(c, 10000)
	c.TickLater()

	run(t, sim)

	wantTicks(t, c, []timing.VTimeInPicoSec{1000, 10000}, []timing.VTimeInPicoSec{10000})
}

// TestASupersededTickIsNotDuplicated is the case where an earlier tick
// supersedes a pending one and then asks for it again: the component must
// still tick once on that edge.
func TestASupersededTickIsNotDuplicated(t *testing.T) {
	sim := newSim()
	c := newTimerSim(t, sim)

	addWork(c, 5000)
	scheduleCall(sim, "Poke", 1500, c.TickLater) // ticks at 2000, which asks for 5000 again

	run(t, sim)

	wantTicks(t, c, []timing.VTimeInPicoSec{2000, 5000}, []timing.VTimeInPicoSec{5000})
}

// TestTickAtDuringATickIsNotLost shows that the tick being handled does not
// swallow a request it makes for a later edge.
func TestTickAtDuringATickIsNotLost(t *testing.T) {
	sim := newSim()
	c := newTimerSim(t, sim)

	c.State.AddOnTick = []timing.VTimeInPicoSec{7000}
	c.TickLater()

	run(t, sim)

	wantTicks(t, c, []timing.VTimeInPicoSec{1000, 7000}, []timing.VTimeInPicoSec{7000})
}

// TestTimedWorkIsSerializedUnderTheParallelEngine wakes components from many
// handlers at the times of their timed work. The parallel engine runs those
// events concurrently with the ticks; each component still ticks once per
// edge and does all its work. Run with -race to check the locking.
func TestTimedWorkIsSerializedUnderTheParallelEngine(t *testing.T) {
	sim := akitasim.MakeBuilder().WithParallelEngine().Build()

	const n = 8
	comps := make([]*timerComp, n)
	for i := range comps {
		comps[i] = buildTimer(sim, fmt.Sprintf("T%d", i))
	}
	for i := range n {
		sim.Engine().RegisterHandler(fmt.Sprintf("Poke%d", i), callHandler{})
	}
	if err := sim.Initialize(); err != nil {
		t.Fatal(err)
	}

	due := []timing.VTimeInPicoSec{3000, 5000, 8000}
	for _, c := range comps {
		for _, d := range due {
			addWork(c, d)
		}
	}
	for i := range n {
		for _, d := range due {
			for _, c := range comps {
				scheduleCall(sim, fmt.Sprintf("Poke%d", i), d, func() {
					c.TickAt(d)
					c.TickLater()
				})
			}
		}
	}

	run(t, sim)

	for _, c := range comps {
		if !slices.IsSorted(c.State.Ticks) ||
			len(slices.Compact(slices.Clone(c.State.Ticks))) != len(c.State.Ticks) {
			t.Errorf("%s ticked at %v, want each edge at most once",
				c.Name(), c.State.Ticks)
		}
		if !reflect.DeepEqual(c.State.Done, due) {
			t.Errorf("%s did work at %v, want %v", c.Name(), c.State.Done, due)
		}
	}
}

func run(t *testing.T, sim *akitasim.Simulation) {
	t.Helper()

	if err := sim.Engine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
