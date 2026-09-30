package wakeup_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/wakeup"
	"github.com/sarchlab/akita/v5/timing"
)

type Spec struct {
	Work int `json:"work"`
}

type State struct {
	Work  int                     `json:"work"`
	Woken []timing.VTimeInPicoSec `json:"woken"`
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

	if m.comp.State.Work == 0 {
		return false
	}

	m.comp.State.Work--

	return true
}

var Definition = wakeup.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{Work: 2},
	NewState: func(c *Comp) State {
		return State{Work: c.Spec().Work}
	},
	NewMiddlewares: func(c *Comp) Middlewares {
		return Middlewares{Worker: &workMW{comp: c}}
	},
}

func build(sim timing.Simulation, spec Spec) *Comp {
	return Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithPorts(Ports{In: messaging.NewPort(nil, 4, 4, "C.In")}).
		Build("C")
}

func newSim() timing.Simulation {
	return modeling.NewStandaloneSimulation(timing.NewSerialEngine())
}

func run(t *testing.T, sim timing.Simulation) {
	t.Helper()

	if err := sim.GetEngine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunsWhileMiddlewaresMakeProgress(t *testing.T) {
	sim := newSim()
	c := build(sim, Definition.DefaultSpec)

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
	c.WakeAt(100) // earlier than the pending one: scheduled too
	c.WakeAt(300) // later than a pending one: dropped
	run(t, sim)

	want := []timing.VTimeInPicoSec{100, 200}
	if !reflect.DeepEqual(c.State.Woken, want) {
		t.Errorf("woken at %v, want %v", c.State.Woken, want)
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	c := build(newSim(), Definition.DefaultSpec)
	c.State.Work = 7

	var buf bytes.Buffer
	if err := c.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	restored := build(newSim(), Definition.DefaultSpec)
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
