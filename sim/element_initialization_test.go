package sim_test

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type elementSpec struct {
	Freq    timing.Freq
	Initial int
}
type elementProbe struct {
	Calls  []string
	FailAt string
}
type elementResources struct{ Probe *elementProbe }
type elementPorts struct{ Top messaging.Port }
type eventState struct{ Value int }
type tickState struct{ Value int }
type wakeState struct{ Value int }
type eventMiddleware struct{ Initial int }

func (*eventMiddleware) Handle(timing.Event) {}

type eventMiddlewares struct{ Work *eventMiddleware }
type tickMiddlewares struct{ Work *elementMiddleware }
type wakeMiddlewares struct{ Work *elementMiddleware }
type elementMiddleware struct{ Initial int }

func (*elementMiddleware) Handle(timing.Event) bool { return false }

type eventComp = event.Component[elementSpec, eventState, elementResources, elementPorts, eventMiddlewares]
type tickComp = ticking.Component[elementSpec, tickState, elementResources, elementPorts, tickMiddlewares]
type wakeComp = wakeup.Component[elementSpec, wakeState, elementResources, elementPorts, wakeMiddlewares]

func (p *elementProbe) state(name string, bound bool, value int) int {
	if !bound {
		panic("initializer ran before binding")
	}
	p.Calls = append(p.Calls, name+".State")
	if p.FailAt == name {
		panic("invalid component configuration")
	}
	return value
}
func (p *elementProbe) middleware(name string, value int) *elementMiddleware {
	p.Calls = append(p.Calls, name+".Middlewares")
	return &elementMiddleware{Initial: value}
}
func (s *eventState) Initialize(c *eventComp) {
	s.Value = c.Resources.Probe.state(c.Name(), c.Ports.Top != nil, c.Spec.Initial)
}
func (s *tickState) Initialize(c *tickComp) {
	s.Value = c.Resources.Probe.state(c.Name(), c.Ports.Top != nil, c.Spec.Initial)
}
func (s *wakeState) Initialize(c *wakeComp) {
	s.Value = c.Resources.Probe.state(c.Name(), c.Ports.Top != nil, c.Spec.Initial)
}
func (m *eventMiddlewares) Initialize(c *eventComp) {
	c.Resources.Probe.Calls = append(c.Resources.Probe.Calls, c.Name()+".Middlewares")
	m.Work = &eventMiddleware{Initial: c.State.Value}
}
func (m *tickMiddlewares) Initialize(c *tickComp) {
	m.Work = c.Resources.Probe.middleware(c.Name(), c.State.Value)
}
func (m *wakeMiddlewares) Initialize(c *wakeComp) {
	m.Work = c.Resources.Probe.middleware(c.Name(), c.State.Value)
}

func buildElements(s *sim.Simulation, p *elementProbe) (*eventComp, *tickComp, *wakeComp) {
	spec := elementSpec{Freq: timing.GHz, Initial: 7}
	resources := elementResources{Probe: p}
	ed := modeling.MakeBuilder[*eventComp](spec)
	td := modeling.MakeBuilder[*tickComp](spec)
	wd := modeling.MakeBuilder[*wakeComp](spec)
	return ed.WithSimulation(s).WithResources(resources).Build("Event"),
		td.WithSimulation(s).WithResources(resources).Build("Ticking"),
		wd.WithSimulation(s).WithResources(resources).Build("Wakeup")
}

func TestElementInitializationOrderAndRestore(t *testing.T) {
	s := sim.MakeBuilder().Build()
	p := &elementProbe{}
	e, tc, w := buildElements(s, p)
	require.Empty(t, p.Calls)
	require.ErrorContains(t, s.Initialize(), "Top")
	require.Empty(t, p.Calls)
	e.BindPort("Top", twowaybuffered.NewPort(1, 1))
	tc.BindPort("Top", twowaybuffered.NewPort(1, 1))
	w.BindPort("Top", twowaybuffered.NewPort(1, 1))
	require.NoError(t, s.Initialize())
	want := []string{
		"Event.State", "Event.Middlewares", "Ticking.State", "Ticking.Middlewares",
		"Wakeup.State", "Wakeup.Middlewares",
	}
	require.Equal(t, want, p.Calls)
	for _, initial := range []int{e.Middlewares.Work.Initial, tc.Middlewares.Work.Initial, w.Middlewares.Work.Initial} {
		require.Equal(t, 7, initial)
	}
	var checkpoint bytes.Buffer
	require.NoError(t, e.SaveCheckpoint(&checkpoint))
	e.State.Value = 99
	require.NoError(t, e.LoadCheckpoint(&checkpoint))
	require.Equal(t, 7, e.State.Value)
	require.Error(t, s.Initialize())
	require.Equal(t, want, p.Calls)
	require.NoError(t, s.Run())
	t.Logf("Build called no initializers; missing ports remained repairable; initialized in order %v; "+
		"checkpoint restore and repeated Initialize did not rerun methods", p.Calls)
}

func TestElementInitializationFailureRequiresRebuild(t *testing.T) {
	s := sim.MakeBuilder().Build()
	p := &elementProbe{FailAt: "Ticking"}
	e, tc, w := buildElements(s, p)
	e.BindPort("Top", twowaybuffered.NewPort(1, 1))
	tc.BindPort("Top", twowaybuffered.NewPort(1, 1))
	w.BindPort("Top", twowaybuffered.NewPort(1, 1))
	require.PanicsWithValue(t, "invalid component configuration", func() { _ = s.Initialize() })
	require.Equal(t, []string{"Event.State", "Event.Middlewares", "Ticking.State"}, p.Calls)
	require.Zero(t, tc.State.Value)
	require.Nil(t, tc.Middlewares.Work)
	require.Zero(t, w.State.Value)
	require.Error(t, s.Initialize())
	require.Error(t, s.Run())
	require.Panics(t, func() { e.BindPort("Top", twowaybuffered.NewPort(1, 1)) })
	t.Log("A component-specific panic stopped later initialization and rejected retry, execution, and further binding")
}

func TestZeroElementsNeedNoInitializers(t *testing.T) {
	s := sim.MakeBuilder().Build()
	def := modeling.MakeBuilder[*event.Component[
		modeling.None, modeling.None, modeling.None, modeling.None, modeling.None]](modeling.None{})
	c := def.WithSimulation(s).Build("Zero")
	require.NoError(t, s.Initialize())
	require.Equal(t, modeling.None{}, c.State)
	require.Equal(t, modeling.None{}, c.Middlewares)
	require.NoError(t, s.Run())
	t.Log("A zero Spec, State and Middlewares component initializes and runs without callbacks or methods")
}
