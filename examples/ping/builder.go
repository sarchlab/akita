package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for a ping component.
var defaultSpec = Spec{}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds ping components. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithRegistrar. The component declares
// its "Out" port; the port instance is supplied externally after Build with
// AssignPort (the caller chooses the buffer size).
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
}

// MakeBuilder creates a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// Build creates a new ping component with the given name. It declares the
// component's "Out" port; assign the port instance after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("ping: WithRegistrar is required")
	}

	comp := modeling.NewEventDrivenBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithSpec(b.spec).
		WithProcessor(&pingProcessor{}).
		Build(name)

	comp.DeclarePort("Out", pingPeer)

	b.registrar.RegisterComponent(comp)

	return comp
}

// SchedulePing schedules a ping to be sent at the given time to the given
// destination.
func SchedulePing(
	comp *Comp,
	sendAt timing.VTimeInPicoSec,
	dst messaging.RemotePort,
) {
	state := &comp.State
	state.ScheduledPings = append(state.ScheduledPings, scheduledPing{
		SendAt: sendAt,
		Dst:    dst,
	})
	comp.ScheduleWakeAt(sendAt)
}
