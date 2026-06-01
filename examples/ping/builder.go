package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Builder builds ping components.
type Builder struct {
	engine    timing.EventScheduler
	registrar modeling.Registrar
	outPort   messaging.Port
}

// MakeBuilder creates a new Builder.
func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets the simulation engine.
func (b Builder) WithEngine(engine timing.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithSimulation wires the builder to a simulation. It sources the engine from
// the simulation and registers the built component with it.
func (b Builder) WithSimulation(sim modeling.Registrar) Builder {
	b.registrar = sim
	b.engine = sim.GetEngine()
	return b
}

// WithOutPort sets the output port.
func (b Builder) WithOutPort(port messaging.Port) Builder {
	b.outPort = port
	return b
}

// Build creates a new ping component with the given name.
func (b Builder) Build(name string) *Comp {
	comp := modeling.NewEventDrivenBuilder[PingSpec, PingState, modeling.None]().
		WithEngine(b.engine).
		WithSpec(PingSpec{OutPort: b.outPort}).
		WithProcessor(&PingProcessor{}).
		Build(name)

	b.outPort.SetComponent(comp)

	if b.registrar != nil {
		b.registrar.RegisterComponent(comp)
	}

	return comp
}

// SchedulePing schedules a ping to be sent at the given time to the given
// destination.
func SchedulePing(
	comp *Comp,
	sendAt timing.VTimeInSec,
	dst messaging.RemotePort,
) {
	state := &comp.State
	state.ScheduledPings = append(state.ScheduledPings, ScheduledPing{
		SendAt: sendAt,
		Dst:    dst,
	})
	comp.ScheduleWakeAt(sendAt)
}
