package ping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// Comp is the ping component built on EventDrivenComponent.
type Comp = modeling.EventDrivenComponent[PingSpec, PingState]

// Builder builds ping components.
type Builder struct {
	engine  sim.EventScheduler
	outPort sim.Port
}

// MakeBuilder creates a new Builder.
func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets the simulation engine.
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithOutPort sets the output port.
func (b Builder) WithOutPort(port sim.Port) Builder {
	b.outPort = port
	return b
}

// Build creates a new ping component with the given name.
func (b Builder) Build(name string) *Comp {
	comp := modeling.NewEventDrivenBuilder[PingSpec, PingState]().
		WithEngine(b.engine).
		WithSpec(PingSpec{OutPort: b.outPort}).
		WithProcessor(&PingProcessor{}).
		Build(name)

	b.outPort.SetComponent(comp)

	return comp
}

// SchedulePing schedules a ping to be sent at the given time to the given
// destination.
func SchedulePing(
	comp *Comp,
	sendAt sim.VTimeInSec,
	dst sim.RemotePort,
) {
	state := comp.GetStatePtr()
	state.ScheduledPings = append(state.ScheduledPings, ScheduledPing{
		SendAt: sendAt,
		Dst:    dst,
	})
	comp.ScheduleWakeAt(sendAt)
}
