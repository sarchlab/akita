package event_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/event"
	"github.com/sarchlab/akita/v5/timing"
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
				EventBase: m.comp.MakeEventBase(e.Time() + m.comp.Spec().Latency),
				ReqID:     msg.Meta().ID,
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
	messaging.MsgMeta
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

func TestRecvAndScheduledEventsReachTheMiddlewares(t *testing.T) {
	sim := newSim()
	c := build(sim, Definition.DefaultSpec)

	c.Ports.In.Deliver(req{messaging.MsgMeta{ID: 1}})
	c.Ports.In.Deliver(req{messaging.MsgMeta{ID: 2}})

	if err := sim.GetEngine().Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []done{{ID: 1, At: 10}, {ID: 2, At: 10}}
	if !reflect.DeepEqual(c.State.Done, want) {
		t.Errorf("done %v, want %v", c.State.Done, want)
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
