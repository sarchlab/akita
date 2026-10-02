package base_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/internal/base"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

type baseSpec struct {
	Size int `json:"size"`
}

type basePorts struct {
	In    messaging.Port
	Links []messaging.Port
}

// baseComp is the smallest component built on ComponentBase.
type baseComp struct {
	base.ComponentBase[baseSpec, modeling.None, modeling.None, basePorts, modeling.None]
}

func (c *baseComp) NotifyRecv(messaging.Port)     {}
func (c *baseComp) NotifyPortFree(messaging.Port) {}
func (c *baseComp) Handle(timing.Event)           {}

// recordingSim records the ports and components registered with it.
type recordingSim struct {
	timing.Simulation
	ports      []string
	components []string
}

func (s *recordingSim) RegisterPort(p naming.Named) {
	s.ports = append(s.ports, p.Name())
}

func (s *recordingSim) RegisterComponent(c naming.Named) {
	s.components = append(s.components, c.Name())
}

func newRecordingSim() *recordingSim {
	return &recordingSim{
		Simulation: modeling.NewStandaloneSimulation(timing.NewSerialEngine()),
	}
}

func buildBase(sim timing.Simulation, ports basePorts) *baseComp {
	c := &baseComp{}
	base.Init(&c.ComponentBase, c,
		sim, "C", baseSpec{Size: 4}, modeling.None{}, ports)

	return c
}

func unowned(name string) messaging.Port {
	return messaging.NewPort(name, 1, 1)
}

func expectPanic(t *testing.T, substr string, f func()) {
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

func TestInitComponentBaseBindsEveryPortAndRegisterRegisters(t *testing.T) {
	sim := newRecordingSim()
	in, l0, l1 := unowned("C.In"), unowned("C.Links[0]"), unowned("C.Links[1]")

	c := buildBase(sim, basePorts{In: in, Links: []messaging.Port{l0, l1}})

	for _, p := range []messaging.Port{in, l0, l1} {
		if p.Owner() != modeling.Component(c) {
			t.Errorf("port %s is not bound to the component", p.Name())
		}
	}

	if c.Ports.In != in || c.Ports.Links[1] != l1 {
		t.Errorf("ports are not reachable through the Ports fields")
	}

	if len(sim.ports) != 0 || len(sim.components) != 0 {
		t.Errorf("InitComponentBase registered %v and %v, want nothing before Register",
			sim.ports, sim.components)
	}

	base.Register(&c.ComponentBase)

	want := []string{"C.In", "C.Links[0]", "C.Links[1]"}
	if !reflect.DeepEqual(sim.ports, want) {
		t.Errorf("registered ports %v, want %v", sim.ports, want)
	}

	if !reflect.DeepEqual(sim.components, []string{"C"}) {
		t.Errorf("registered components %v, want [C]", sim.components)
	}
}

func TestInitComponentBaseRejectsMisconfiguredPorts(t *testing.T) {
	t.Run("missing port", func(t *testing.T) {
		expectPanic(t, "port In is not given", func() {
			buildBase(newRecordingSim(), basePorts{})
		})
	})

	t.Run("misnamed port", func(t *testing.T) {
		expectPanic(t, `want "C.In"`, func() {
			buildBase(newRecordingSim(), basePorts{In: unowned("Other.In")})
		})
	})

	t.Run("misnamed group member", func(t *testing.T) {
		expectPanic(t, `want "C.Links[0]"`, func() {
			buildBase(newRecordingSim(), basePorts{
				In:    unowned("C.In"),
				Links: []messaging.Port{unowned("C.Links")},
			})
		})
	})

	t.Run("port of another component", func(t *testing.T) {
		in := unowned("C.In")
		buildBase(newRecordingSim(), basePorts{In: in})

		expectPanic(t, "already belongs to", func() {
			buildBase(newRecordingSim(), basePorts{In: in})
		})
	})

	t.Run("non-port field", func(t *testing.T) {
		type badPorts struct {
			N  int
			In messaging.Port
		}

		expectPanic(t, "must be an exported messaging.Port or []messaging.Port", func() {
			b := &base.ComponentBase[
				baseSpec, modeling.None, modeling.None, badPorts, modeling.None]{}
			base.Init(b, &baseComp{},
				newRecordingSim(), "C", baseSpec{}, modeling.None{},
				badPorts{In: unowned("C.In")})
		})
	})
}

func TestNameIsTheInstanceAndSpecIsKept(t *testing.T) {
	c := buildBase(newRecordingSim(), basePorts{In: unowned("C.In")})

	if c.Name() != "C" {
		t.Errorf("Name() = %q, want the instance name C", c.Name())
	}

	if c.Spec.Size != 4 {
		t.Errorf("Spec = %+v, want Size 4", c.Spec)
	}
}

type listSpec struct {
	Targets []string `json:"targets"`
}

// listComp is a component whose Spec holds a slice.
type listComp struct {
	base.ComponentBase[listSpec, modeling.None, modeling.None, modeling.None, modeling.None]
}

func (c *listComp) NotifyRecv(messaging.Port)     {}
func (c *listComp) NotifyPortFree(messaging.Port) {}
func (c *listComp) Handle(timing.Event)           {}

func TestInitGivesTheInstanceItsOwnSpecSlices(t *testing.T) {
	spec := listSpec{Targets: []string{"A", "B"}}

	c := &listComp{}
	base.Init(&c.ComponentBase, c, newRecordingSim(), "C",
		spec, modeling.None{}, modeling.None{})

	spec.Targets[0] = "X"

	if got := c.Spec.Targets; !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("Spec.Targets = %v after the caller changed its slice, want [A B]", got)
	}
}
