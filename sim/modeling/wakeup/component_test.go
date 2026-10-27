package wakeup_test

import (
	"bytes"
	"reflect"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type Spec struct {
	Work int `json:"work"`
}

type State struct {
	Work  int                     `json:"work"`
	Woken []timing.VTimeInPicoSec `json:"woken"`

	// WakeAgainAt, when set, is a time the component asks to wake at on every
	// earlier wakeup, as for work due then.
	WakeAgainAt timing.VTimeInPicoSec `json:"wake_again_at"`

	// WakeOnceAt, when set, is a time the first wakeup asks for, once.
	WakeOnceAt timing.VTimeInPicoSec `json:"wake_once_at"`
}

type Ports struct {
	In messaging.Port
}

type Middlewares struct {
	Worker *workMW
}

type Comp = wakeup.Component[Spec, State, modeling.None, Ports, Middlewares]

// workMW records every wakeup and makes progress while work is left,
// doing one unit per wakeup.
type workMW struct {
	comp *Comp
}

func (m *workMW) Handle(e timing.Event) bool {
	m.comp.State.Woken = append(m.comp.State.Woken, e.Time())

	if again := m.comp.State.WakeAgainAt; again > e.Time() {
		m.comp.WakeAt(again)
	}

	if once := m.comp.State.WakeOnceAt; once != 0 {
		m.comp.State.WakeOnceAt = 0
		m.comp.WakeAt(once)
	}

	if m.comp.State.Work == 0 {
		return false
	}

	m.comp.State.Work--

	return true
}

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{Work: 2}
}

func build(sim *akitasim.Simulation, spec Spec) *Comp {
	builtComponent := modeling.MakeBuilder[*Comp](DefaultSpec()).
		WithSimulation(sim).
		WithSpec(spec).
		Build("C")

	builtComponent.BindPort("In", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
		Build())
	if err := sim.Initialize(); err != nil {
		panic(err)
	}
	return builtComponent
}

func newSim() *akitasim.Simulation {
	return akitasim.MakeBuilder().Build()
}

func run(t *testing.T, sim *akitasim.Simulation) {
	t.Helper()

	if err := sim.Engine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunsWhileMiddlewaresMakeProgress(t *testing.T) {
	sim := newSim()
	c := build(sim, DefaultSpec())

	c.NotifyRecv(c.Ports.In)
	run(t, sim)

	// Two wakeups do the two units of work; the third finds none, and the
	// component sleeps. All happen at the time it was woken.
	want := []timing.VTimeInPicoSec{0, 0, 0}
	if c.State.Work != 0 || !reflect.DeepEqual(c.State.Woken, want) {
		t.Errorf("Work = %d, woken at %v; want 0 and %v",
			c.State.Work, c.State.Woken, want)
	}
}

func TestWakeAtWakesAtThatTime(t *testing.T) {
	sim := newSim()
	c := build(sim, Spec{})

	c.WakeAt(200)
	c.WakeAt(100) // earlier than a pending one: scheduled
	c.WakeAt(300) // later than a pending one: scheduled
	c.WakeAt(200) // at a pending time: dropped
	run(t, sim)

	want := []timing.VTimeInPicoSec{100, 200, 300}
	if !reflect.DeepEqual(c.State.Woken, want) {
		t.Errorf("woken at %v, want %v", c.State.Woken, want)
	}
}

// TestASupersededWakeupIsNotDuplicated covers a pending wakeup that an
// earlier one superseded: when the earlier wakeup asks for it again, the
// component still wakes once at that time.
func TestASupersededWakeupIsNotDuplicated(t *testing.T) {
	sim := newSim()
	c := build(sim, Spec{})
	c.State.WakeAgainAt = 300

	c.WakeAt(300)
	c.WakeAt(100)
	run(t, sim)

	want := []timing.VTimeInPicoSec{100, 300}
	if !reflect.DeepEqual(c.State.Woken, want) {
		t.Errorf("woken at %v, want %v", c.State.Woken, want)
	}
}

// TestWakeAtDuringAWakeupIsNotLost shows that a request made once during a
// wakeup fires, though a wakeup is pending earlier and no later wakeup asks
// again.
func TestWakeAtDuringAWakeupIsNotLost(t *testing.T) {
	sim := newSim()
	c := build(sim, Spec{})
	c.State.WakeOnceAt = 300

	c.WakeAt(100)
	c.WakeAt(200)
	run(t, sim)

	want := []timing.VTimeInPicoSec{100, 200, 300}
	if !reflect.DeepEqual(c.State.Woken, want) {
		t.Errorf("woken at %v, want %v", c.State.Woken, want)
	}
}

// TestWakeAtWhileAnEarlierWakeupIsPending shows that a later request is not
// absorbed by a pending earlier wakeup.
func TestWakeAtWhileAnEarlierWakeupIsPending(t *testing.T) {
	sim := newSim()
	c := build(sim, Spec{})

	c.WakeAt(100)
	c.WakeAt(300)
	run(t, sim)

	want := []timing.VTimeInPicoSec{100, 300}
	if !reflect.DeepEqual(c.State.Woken, want) {
		t.Errorf("woken at %v, want %v", c.State.Woken, want)
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	c := build(newSim(), DefaultSpec())
	c.State.Work = 7

	var buf bytes.Buffer
	if err := c.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	restored := build(newSim(), DefaultSpec())
	if err := restored.LoadCheckpoint(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if restored.State.Work != 7 {
		t.Errorf("restored Work = %d, want 7", restored.State.Work)
	}

	other := build(newSim(), Spec{Work: 3})
	if err := other.LoadCheckpoint(bytes.NewReader(buf.Bytes())); err == nil {
		t.Errorf("LoadCheckpoint into a different Spec succeeded, want an error")
	}
}

func (s *State) Initialize(c *Comp) {
	*s = State{Work: c.Spec.Work}
}

func (m *Middlewares) Initialize(c *Comp) {
	*m = Middlewares{Worker: &workMW{comp: c}}
}
