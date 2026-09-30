package ticking_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
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

// recordMW appends its name to the State log on every tick.
type recordMW struct {
	comp     *Comp
	name     string
	progress bool
}

func (m *recordMW) Tick() bool {
	m.comp.State.Log = append(m.comp.State.Log, m.name)
	return m.progress
}

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{Freq: 1 * timing.GHz, Depth: 2},
	NewState: func(_ string, spec Spec) State {
		return State{Count: spec.Depth}
	},
	NewMiddlewares: func(c *Comp) Middlewares {
		return Middlewares{
			First:  &recordMW{comp: c, name: "first"},
			Second: &recordMW{comp: c, name: "second", progress: true},
		}
	},
}

func newSim() timing.Simulation {
	return modeling.NewStandaloneSimulation(timing.NewSerialEngine())
}

func newPort(name string) messaging.Port {
	return messaging.NewPort(nil, 1, 1, name)
}

func build(ports Ports) *Comp {
	return Definition.Builder().WithSimulation(newSim()).WithPorts(ports).Build("C")
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

func TestBuildBindsPortsAndRunsMiddlewaresInOrder(t *testing.T) {
	in := newPort("C.In")
	c := build(Ports{In: in})

	if in.Component() != messaging.Component(c) {
		t.Errorf("port In is not bound to the component")
	}

	if c.Ports.In != in || c.GetPortByName("In") != in {
		t.Errorf("port In is not reachable through the field and by name")
	}

	if c.State.Count != 2 {
		t.Errorf("NewState was not applied: Count = %d, want 2", c.State.Count)
	}

	if progress := c.Tick(); !progress {
		t.Errorf("Tick() = false, want true: one middleware made progress")
	}

	if want := []string{"first", "second"}; !reflect.DeepEqual(c.State.Log, want) {
		t.Errorf("middlewares ran as %v, want %v", c.State.Log, want)
	}
}

func TestTypeNameIsThePackageAndNameIsTheInstance(t *testing.T) {
	c := build(Ports{In: newPort("C.In")})

	if c.Name() != "C" {
		t.Errorf("Name() = %q, want the instance name C", c.Name())
	}

	want := "github.com/sarchlab/akita/v5/modeling/ticking_test"
	if c.TypeName() != want {
		t.Errorf("TypeName() = %q, want the package path %q", c.TypeName(), want)
	}
}

func TestBuildRejectsMisconfiguredPorts(t *testing.T) {
	t.Run("missing port", func(t *testing.T) {
		mustPanic(t, "port In is not given", func() {
			build(Ports{})
		})
	})

	t.Run("misnamed port", func(t *testing.T) {
		mustPanic(t, `want "C.In"`, func() {
			build(Ports{In: newPort("Other.In")})
		})
	})

	t.Run("no ports after Build", func(t *testing.T) {
		c := build(Ports{In: newPort("C.In")})
		mustPanic(t, "ports are fixed at Build", func() {
			c.AssignPort("In", newPort("C.In"))
		})
	})

	t.Run("unknown port name", func(t *testing.T) {
		c := build(Ports{In: newPort("C.In")})
		mustPanic(t, "ports are In, Links[]", func() {
			c.GetPortByName("Out")
		})
	})

	t.Run("no simulation", func(t *testing.T) {
		mustPanic(t, "WithSimulation is required", func() {
			Definition.Builder().Build("C")
		})
	})
}

func TestPortGroupGrowsAfterBuild(t *testing.T) {
	in := newPort("C.In")
	c := build(Ports{In: in})

	link := newPort("C.Links[0]")
	if name := c.AssignPortToGroup("Links", link); name != "Links[0]" {
		t.Errorf("AssignPortToGroup returned %q, want Links[0]", name)
	}

	if link.Component() != messaging.Component(c) {
		t.Errorf("group member is not bound to the component")
	}

	if c.NumPortsInGroup("Links") != 1 || c.Ports.Links[0] != link ||
		c.GetPortByName("Links[0]") != link {
		t.Errorf("group member is not reachable")
	}

	if got := c.AllPorts(); len(got) != 2 || got[0] != in || got[1] != link {
		t.Errorf("AllPorts() = %v, want [In Links[0]]", got)
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
	other := Definition.Builder().
		WithSimulation(newSim()).
		WithSpec(spec).
		WithPorts(Ports{In: newPort("C.In")}).
		Build("C")
	if err := other.LoadCheckpoint(bytes.NewReader(saved)); err == nil {
		t.Errorf("LoadCheckpoint into a different Spec succeeded, want an error")
	}
}

// --- Shape checks on the Ports and Middlewares types ---

type badPorts struct {
	In messaging.Port
	N  int
}

type oneMiddleware struct {
	Only *recordMW
}

func TestBuildRejectsBadShapes(t *testing.T) {
	t.Run("non-port field in Ports", func(t *testing.T) {
		def := ticking.Definition[Spec, State, modeling.None, badPorts, oneMiddleware]{
			DefaultSpec: Spec{Freq: 1 * timing.GHz},
			NewMiddlewares: func(
				*ticking.Component[Spec, State, modeling.None, badPorts, oneMiddleware],
			) oneMiddleware {
				return oneMiddleware{Only: &recordMW{}}
			},
		}

		mustPanic(t, "must be messaging.Port or []messaging.Port", func() {
			def.Builder().
				WithSimulation(newSim()).
				WithPorts(badPorts{In: newPort("B.In")}).
				Build("B")
		})
	})

	t.Run("unset middleware", func(t *testing.T) {
		def := Definition
		def.NewMiddlewares = func(c *Comp) Middlewares {
			return Middlewares{First: &recordMW{comp: c}}
		}

		mustPanic(t, "Second is not set by NewMiddlewares", func() {
			def.Builder().
				WithSimulation(newSim()).
				WithPorts(Ports{In: newPort("C.In")}).
				Build("C")
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
			def.Builder().
				WithSimulation(newSim()).
				WithPorts(Ports{In: newPort("C.In")}).
				Build("C")
		})
	})
}
