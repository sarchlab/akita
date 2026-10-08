package base_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/internal/base"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
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
	s.Simulation.RegisterPort(p)
	s.ports = append(s.ports, p.Name())
}

func (s *recordingSim) RegisterComponent(c naming.Named) {
	s.Simulation.RegisterComponent(c)
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
		sim, "C", baseSpec{Size: 4}, modeling.None{})
	base.Register(&c.ComponentBase)
	if ports.In != nil {
		c.BindPort("In", ports.In)
	}
	for i, p := range ports.Links {
		c.BindPort(fmt.Sprintf("Links[%d]", i), p)
	}

	return c
}

func unowned(name string) messaging.Port {
	return twowaybuffered.NewPort(1, 1)
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

func TestBindPortEstablishesBothSides(t *testing.T) {
	s := newRecordingSim()
	c := buildBase(s, basePorts{})
	if c.Ports.In != nil || len(s.ports) != 0 {
		t.Fatal("Build must leave ports unbound")
	}
	in, link := unowned(""), unowned("")
	c.BindPort("In", in)
	c.BindPort("Links[0]", link)
	if c.Ports.In != in || in.Owner() != c || in.Name() != "C.In" || link.Name() != "C.Links[0]" {
		t.Fatal("binding did not establish both sides")
	}
	if !reflect.DeepEqual(s.ports, []string{"C.In", "C.Links[0]"}) {
		t.Fatal(s.ports)
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	expectPanic(t, "frozen", func() { c.BindPort("Links[1]", unowned("")) })
}
func TestInvalidBindingDoesNotMutate(t *testing.T) {
	for _, slot := range []string{"Missing", "Links", "Links[-1]", "Links[01]", "In[0]"} {
		t.Run(slot, func(t *testing.T) {
			s := newRecordingSim()
			c := buildBase(s, basePorts{})
			p := unowned("")
			expectPanic(t, "", func() { c.BindPort(slot, p) })
			if p.Owner() != nil || p.Name() != "" || len(s.ports) != 0 {
				t.Fatal("invalid binding mutated port")
			}
		})
	}
	s := newRecordingSim()
	c := buildBase(s, basePorts{In: unowned("")})
	replacement := unowned("")
	expectPanic(t, "already", func() { c.BindPort("In", replacement) })
	if replacement.Owner() != nil {
		t.Fatal("replacement was partially bound")
	}
}
func TestInitializationValidatesBeforeFreezing(t *testing.T) {
	s := newRecordingSim()
	c := buildBase(s, basePorts{})
	calls := 0
	base.SetInitializer(&c.ComponentBase, func() { calls++ })
	if err := s.Initialize(); err == nil || !strings.Contains(err.Error(), "In") {
		t.Fatalf("missing port: %v", err)
	}
	if calls != 0 {
		t.Fatal("initializer ran before validation")
	}
	c.BindPort("In", unowned(""))
	c.BindPort("Links[1]", unowned(""))
	if err := s.Initialize(); err == nil {
		t.Fatal("slice hole accepted")
	}
	c.BindPort("Links[0]", unowned(""))
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	if err := s.Initialize(); err == nil {
		t.Fatal("double initialization accepted")
	}
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
		spec, modeling.None{})

	spec.Targets[0] = "X"

	if got := c.Spec.Targets; !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("Spec.Targets = %v after the caller changed its slice, want [A B]", got)
	}
}
