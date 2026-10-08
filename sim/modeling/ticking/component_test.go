package ticking_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// --- A small ticking component ---

type Spec struct {
	Freq  timing.Freq `json:"freq"`
	Depth int         `json:"depth"`
}

type State struct {
	Count int      `json:"count"`
	Log   []string `json:"log"`
}

type Ports struct {
	In    messaging.Port
	Links []messaging.Port
}

type Middlewares struct {
	First  *recordMW
	Second *recordMW
}

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// recordMW logs its name and the type of every event it handles. A counting
// middleware makes progress while State.Count is positive, counting it down.
type recordMW struct {
	comp     *Comp
	name     string
	counting bool
}

func (m *recordMW) Handle(e timing.Event) bool {
	m.comp.State.Log = append(m.comp.State.Log, fmt.Sprintf("%s %T", m.name, e))

	if !m.counting || m.comp.State.Count == 0 {
		return false
	}

	m.comp.State.Count--

	return true
}

// pokeEvent stands for an event other than a tick: Handle passes whatever it
// receives to the middlewares.
type pokeEvent struct {
	timing.EventBase
}

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{Freq: 1 * timing.GHz, Depth: 2},
	NewState: func(c *Comp) State {
		return State{Count: c.Spec.Depth}
	},
	NewMiddlewares: func(c *Comp) Middlewares {
		return Middlewares{
			First:  &recordMW{comp: c, name: "first"},
			Second: &recordMW{comp: c, name: "second", counting: true},
		}
	},
}

func newSim() timing.Simulation {
	return modeling.NewStandaloneSimulation(timing.NewSerialEngine())
}

func newPort(name string) messaging.Port {
	return twowaybuffered.NewPort(1, 1)
}

func build(ports Ports) *Comp {
	setupSim1 := newSim()
	builtComponent := Definition.Builder().WithSimulation(setupSim1).Build("C")
	builtComponentPorts := ports
	builtComponent.BindPort("In", builtComponentPorts.In)
	for i, p := range builtComponentPorts.Links {
		builtComponent.BindPort(fmt.Sprintf("Links[%d]", i), p)
	}
	if err := setupSim1.Initialize(); err != nil {
		panic(err)
	}

	return builtComponent
}

func mustPanic(t *testing.T, substr string, f func()) {
	t.Helper()

	defer func() {
		t.Helper()

		r := recover()
		if r == nil {
			t.Fatalf("expected a panic containing %q", substr)
		}
		if !strings.Contains(fmt.Sprint(r), substr) {
			t.Fatalf("panic %q does not contain %q", fmt.Sprint(r), substr)
		}
	}()

	f()
}

// --- Tests ---

func TestBuildRunsMiddlewaresInOrder(t *testing.T) {
	in := newPort("C.In")
	c := build(Ports{In: in})

	if in.Owner() != modeling.Component(c) || c.Ports.In != in {
		t.Errorf("port In is not bound to the component")
	}

	if c.State.Count != 2 {
		t.Errorf("NewState was not applied: Count = %d, want 2", c.State.Count)
	}

	c.Handle(ticking.MakeTickEvent(1, "C", 0))

	want := []string{"first ticking.TickEvent", "second ticking.TickEvent"}
	if !reflect.DeepEqual(c.State.Log, want) {
		t.Errorf("middlewares ran as %v, want %v", c.State.Log, want)
	}
}

func TestTicksWhileMiddlewaresMakeProgress(t *testing.T) {
	sim := newSim()
	c := Definition.Builder().
		WithSimulation(sim).
		Build("C")
	cPorts := Ports{In: newPort("C.In")}
	c.BindPort("In", cPorts.In)
	for i, p := range cPorts.Links {
		c.BindPort(fmt.Sprintf("Links[%d]", i), p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	c.NotifyRecv(c.Ports.In)
	if err := sim.Engine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Count starts at 2: two ticks make progress, and the third, which makes
	// none, lets the component sleep.
	if c.State.Count != 0 || len(c.State.Log) != 6 {
		t.Errorf("after the run, Count = %d and %d middleware calls, "+
			"want 0 and 6 (3 ticks)", c.State.Count, len(c.State.Log))
	}
}

func TestHandlePassesEveryEventToTheMiddlewares(t *testing.T) {
	c := build(Ports{In: newPort("C.In")})

	c.Handle(pokeEvent{timing.EventBase{HandlerID_: "C"}})

	want := []string{"first ticking_test.pokeEvent", "second ticking_test.pokeEvent"}
	if !reflect.DeepEqual(c.State.Log, want) {
		t.Errorf("middlewares ran as %v, want %v", c.State.Log, want)
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	c := build(Ports{In: newPort("C.In")})
	c.State.Count = 7

	var buf bytes.Buffer
	if err := c.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	saved := buf.Bytes()

	restored := build(Ports{In: newPort("C.In")})
	if err := restored.LoadCheckpoint(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if restored.State.Count != 7 {
		t.Errorf("restored Count = %d, want 7", restored.State.Count)
	}

	spec := Definition.DefaultSpec
	spec.Depth = 3
	setupSim2 := newSim()
	other := Definition.Builder().
		WithSimulation(setupSim2).
		WithSpec(spec).
		Build("C")
	otherPorts := Ports{In: newPort("C.In")}
	other.BindPort("In", otherPorts.In)
	for i, p := range otherPorts.Links {
		other.BindPort(fmt.Sprintf("Links[%d]", i), p)
	}
	if err := setupSim2.Initialize(); err != nil {
		panic(err)
	}

	if err := other.LoadCheckpoint(bytes.NewReader(saved)); err == nil {
		t.Errorf("LoadCheckpoint into a different Spec succeeded, want an error")
	}
}

// --- Shape checks on the Ports and Middlewares types ---

type oneMiddleware struct {
	Only *recordMW
}

func TestBuildRejectsBadShapes(t *testing.T) {
	t.Run("no simulation", func(t *testing.T) {
		mustPanic(t, "WithSimulation is required", func() {
			Definition.Builder().Build("C")
		})
	})

	t.Run("unset middleware", func(t *testing.T) {
		def := Definition
		def.NewMiddlewares = func(c *Comp) Middlewares {
			return Middlewares{First: &recordMW{comp: c}}
		}

		mustPanic(t, "Second is not set by NewMiddlewares", func() {
			setupSim3 := newSim()
			builtComponent := def.Builder().
				WithSimulation(setupSim3).
				Build("C")
			builtComponentPorts := Ports{In: newPort("C.In")}
			builtComponent.BindPort("In", builtComponentPorts.In)
			for i, p := range builtComponentPorts.Links {
				builtComponent.BindPort(fmt.Sprintf("Links[%d]", i), p)
			}
			if err := setupSim3.Initialize(); err != nil {
				panic(err)
			}
		})
	})

	t.Run("Spec without Freq", func(t *testing.T) {
		type noFreq struct {
			N int `json:"n"`
		}

		def := ticking.Definition[noFreq, State, modeling.None, Ports, oneMiddleware]{
			NewMiddlewares: func(
				*ticking.Component[noFreq, State, modeling.None, Ports, oneMiddleware],
			) oneMiddleware {
				return oneMiddleware{Only: &recordMW{}}
			},
		}

		mustPanic(t, "must have a Freq timing.Freq field", func() {
			setupSim4 := newSim()
			builtComponent := def.Builder().
				WithSimulation(setupSim4).
				Build("C")
			builtComponentPorts := Ports{In: newPort("C.In")}
			builtComponent.BindPort("In", builtComponentPorts.In)
			for i, p := range builtComponentPorts.Links {
				builtComponent.BindPort(fmt.Sprintf("Links[%d]", i), p)
			}
			if err := setupSim4.Initialize(); err != nil {
				panic(err)
			}
		})
	})
}
