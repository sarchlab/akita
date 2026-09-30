package modeling_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// --- A small component in the five-struct model ---

type compSpec struct {
	Freq  timing.Freq `json:"freq"`
	Depth int         `json:"depth"`
}

type compState struct {
	Count int      `json:"count"`
	Log   []string `json:"log"`
}

type compPorts struct {
	In    messaging.Port
	Links []messaging.Port
}

type compMiddlewares struct {
	First  *recordMW
	Second *recordMW
}

type testComp = modeling.Comp[compSpec, compState, modeling.None, compPorts, compMiddlewares]

// recordMW appends its name to the State log on every tick.
type recordMW struct {
	comp     *testComp
	name     string
	progress bool
}

func (m *recordMW) Tick() bool {
	m.comp.State.Log = append(m.comp.State.Log, m.name)
	return m.progress
}

var compDef = modeling.Definition[compSpec, compState, modeling.None, compPorts, compMiddlewares]{
	Name:        "CompTest",
	DefaultSpec: compSpec{Freq: 1 * timing.GHz, Depth: 2},
	NewState: func(_ string, spec compSpec) compState {
		return compState{Count: spec.Depth}
	},
	NewMiddlewares: func(c *testComp) compMiddlewares {
		return compMiddlewares{
			First:  &recordMW{comp: c, name: "first"},
			Second: &recordMW{comp: c, name: "second", progress: true},
		}
	},
}

func newSim() timing.Simulation {
	return modeling.NewStandaloneSimulation(timing.NewSerialEngine())
}

func buildComp(sim timing.Simulation, ports compPorts) *testComp {
	return compDef.Builder().WithSimulation(sim).WithPorts(ports).Build("C")
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

func TestCompBuildBindsPortsAndRunsMiddlewaresInOrder(t *testing.T) {
	in := messaging.NewPort(nil, 1, 1, "C.In")
	c := buildComp(newSim(), compPorts{In: in})

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

func TestCompRejectsMisconfiguredPorts(t *testing.T) {
	t.Run("missing port", func(t *testing.T) {
		mustPanic(t, "port In is not given", func() {
			buildComp(newSim(), compPorts{})
		})
	})

	t.Run("misnamed port", func(t *testing.T) {
		mustPanic(t, `want "C.In"`, func() {
			buildComp(newSim(), compPorts{In: messaging.NewPort(nil, 1, 1, "Other.In")})
		})
	})

	t.Run("no ports after Build", func(t *testing.T) {
		c := buildComp(newSim(), compPorts{In: messaging.NewPort(nil, 1, 1, "C.In")})
		mustPanic(t, "ports are fixed at Build", func() {
			c.AssignPort("In", messaging.NewPort(nil, 1, 1, "C.In"))
		})
	})

	t.Run("unknown port name", func(t *testing.T) {
		c := buildComp(newSim(), compPorts{In: messaging.NewPort(nil, 1, 1, "C.In")})
		mustPanic(t, "ports are In, Links[]", func() {
			c.GetPortByName("Out")
		})
	})

	t.Run("no simulation", func(t *testing.T) {
		mustPanic(t, "WithSimulation is required", func() {
			compDef.Builder().Build("C")
		})
	})
}

func TestCompPortGroupGrowsAfterBuild(t *testing.T) {
	in := messaging.NewPort(nil, 1, 1, "C.In")
	c := buildComp(newSim(), compPorts{In: in})

	link := messaging.NewPort(nil, 1, 1, "C.Links[0]")
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

func TestCompCheckpointRoundTrip(t *testing.T) {
	c := buildComp(newSim(), compPorts{In: messaging.NewPort(nil, 1, 1, "C.In")})
	c.State.Count = 7

	var buf bytes.Buffer
	if err := c.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	saved := buf.Bytes()

	restored := buildComp(newSim(), compPorts{In: messaging.NewPort(nil, 1, 1, "C.In")})
	if err := restored.LoadCheckpoint(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if restored.State.Count != 7 {
		t.Errorf("restored Count = %d, want 7", restored.State.Count)
	}

	spec := compDef.DefaultSpec
	spec.Depth = 3
	other := compDef.Builder().
		WithSimulation(newSim()).
		WithSpec(spec).
		WithPorts(compPorts{In: messaging.NewPort(nil, 1, 1, "C.In")}).
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

func TestCompRejectsBadShapes(t *testing.T) {
	t.Run("non-port field in Ports", func(t *testing.T) {
		def := modeling.Definition[compSpec, compState, modeling.None, badPorts, oneMiddleware]{
			Name:        "Bad",
			DefaultSpec: compSpec{Freq: 1 * timing.GHz},
			NewMiddlewares: func(*modeling.Comp[compSpec, compState, modeling.None, badPorts, oneMiddleware]) oneMiddleware {
				return oneMiddleware{Only: &recordMW{}}
			},
		}

		mustPanic(t, "must be messaging.Port or []messaging.Port", func() {
			def.Builder().
				WithSimulation(newSim()).
				WithPorts(badPorts{In: messaging.NewPort(nil, 1, 1, "B.In")}).
				Build("B")
		})
	})

	t.Run("unset middleware", func(t *testing.T) {
		def := compDef
		def.NewMiddlewares = func(c *testComp) compMiddlewares {
			return compMiddlewares{First: &recordMW{comp: c}}
		}

		mustPanic(t, "Second is not set by NewMiddlewares", func() {
			def.Builder().
				WithSimulation(newSim()).
				WithPorts(compPorts{In: messaging.NewPort(nil, 1, 1, "C.In")}).
				Build("C")
		})
	})
}
