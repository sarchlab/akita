package directconnection

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	// Builder can help building directconnection.
	"github.com/sarchlab/akita/v5/messaging"
)

type Builder struct {
	engine    timing.EventScheduler
	registrar modeling.Registrar
	freq      timing.Freq
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(e timing.EventScheduler) Builder {
	b.engine = e
	return b
}

// WithSimulation wires the builder to a simulation. It sources the engine from
// the simulation and registers the built connection with it.
func (b Builder) WithSimulation(sim modeling.Registrar) Builder {
	b.registrar = sim
	b.engine = sim.GetEngine()
	return b
}

func (b Builder) WithFreq(f timing.Freq) Builder {
	b.freq = f
	return b
}

func (b Builder) Build(name string) *Comp {
	spec := Spec{Freq: b.freq}

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	// DirectConnection uses secondary tick events so it runs after primary
	// components. Replace the primary TickingComponent created by the builder
	// with a secondary one. Since SerialEngine.RegisterHandler overwrites by
	// name, the final registration is for the secondary component. ✓
	modelComp.TickingComponent = modeling.NewSecondaryTickingComponent(
		name, b.engine, b.freq, modelComp)

	mw := &middleware{
		comp: modelComp,
		ports: ports{
			ports:   make([]messaging.Port, 0),
			portMap: make(map[messaging.RemotePort]int),
		},
	}
	modelComp.AddMiddleware(mw)

	conn := &Comp{Component: modelComp}

	if b.registrar != nil {
		b.registrar.RegisterConnection(conn)
	}

	return conn
}
