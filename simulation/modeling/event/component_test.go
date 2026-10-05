package event_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/event"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// A delay line: every request on In completes Latency later.

type Spec struct {
	Latency timing.VTimeInPicoSec `json:"latency"`
}

type done struct {
	ID uint64                `json:"id"`
	At timing.VTimeInPicoSec `json:"at"`
}

type State struct {
	Done []done `json:"done"`
}

type Ports struct {
	In messaging.Port
}

type Middlewares struct {
	Delay *delayMW
}

type Comp = event.Component[Spec, State, modeling.None, Ports, Middlewares]

// doneEvent completes the request with ID ReqID.
type doneEvent struct {
	timing.EventBase
	ReqID uint64 `json:"req_id"`
}

func init() {
	timing.RegisterEvent(doneEvent{})
}

type delayMW struct {
	comp *Comp
}

func (m *delayMW) Handle(e timing.Event) bool {
	switch e := e.(type) {
	case event.Recv:
		if e.Port != m.comp.Ports.In.Name() {
			return false
		}

		for {
			msg, ok := m.comp.Ports.In.RetrieveIncoming()
			if !ok {
				break
			}

			m.comp.Schedule(doneEvent{
				EventBase: m.comp.MakeEventBase(e.Time() + m.comp.Spec.Latency),
				ReqID:     msg.ID,
			})
		}
	case doneEvent:
		m.comp.State.Done = append(m.comp.State.Done,
			done{ID: e.ReqID, At: e.Time()})
	default:
		return false
	}

	return true
}

var Definition = event.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{Latency: 10},
	NewMiddlewares: func(c *Comp) Middlewares {
		return Middlewares{Delay: &delayMW{comp: c}}
	},
}

type req struct {
}

func build(sim timing.Simulation, spec Spec) *Comp {
	return Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithPorts(Ports{In: messaging.NewPort("C.In", 4, 4)}).
		Build("C")
}

func newSim() timing.Simulation {
	return modeling.NewStandaloneSimulation(timing.NewSerialEngine())
}

func TestRecvAndScheduledEventsReachTheMiddlewares(t *testing.T) {
	sim := newSim()
	c := build(sim, Definition.DefaultSpec)

	c.Ports.In.Deliver(messaging.Msg{ID: 1, Payload: req{}})
	c.Ports.In.Deliver(messaging.Msg{ID: 2, Payload: req{}})

	if err := sim.Engine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []done{{ID: 1, At: 10}, {ID: 2, At: 10}}
	if !reflect.DeepEqual(c.State.Done, want) {
		t.Errorf("done %v, want %v", c.State.Done, want)
	}
}

// The parallel engine runs the events of one time concurrently, even when
// they are for the same component. The component must still handle them one
// at a time.
func TestSimultaneousEventsAreHandledOneAtATime(t *testing.T) {
	sim := modeling.NewStandaloneSimulation(timing.NewParallelEngine())
	c := build(sim, Definition.DefaultSpec)

	const n = 1000
	for i := range n {
		c.Schedule(doneEvent{EventBase: c.MakeEventBase(10), ReqID: uint64(i)})
	}

	if err := sim.Engine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(c.State.Done) != n {
		t.Errorf("recorded %d completions, want %d", len(c.State.Done), n)
	}
}

func TestScheduleRejectsAnotherHandlersEvent(t *testing.T) {
	c := build(newSim(), Definition.DefaultSpec)

	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), `cannot schedule an event for "D"`) {
			t.Errorf("recovered %v, want a panic about scheduling for D", r)
		}
	}()

	c.Schedule(doneEvent{EventBase: timing.MakeEventBase(1, 0, "D")})
}

func TestCheckpointRoundTrip(t *testing.T) {
	c := build(newSim(), Definition.DefaultSpec)
	c.State.Done = []done{{ID: 7, At: 3}}

	var buf bytes.Buffer
	if err := c.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	restored := build(newSim(), Definition.DefaultSpec)
	if err := restored.LoadCheckpoint(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if !reflect.DeepEqual(restored.State, c.State) {
		t.Errorf("restored State %+v, want %+v", restored.State, c.State)
	}

	other := build(newSim(), Spec{Latency: 20})
	if err := other.LoadCheckpoint(bytes.NewReader(buf.Bytes())); err == nil {
		t.Errorf("LoadCheckpoint into a different Spec succeeded, want an error")
	}
}

var _ = messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{req{}}})
