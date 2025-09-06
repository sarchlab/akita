package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/sim"
    "github.com/sarchlab/akita/v4/simv5"
)

// Builder constructs a Comp either from a Spec or per-field setters.
type Builder struct {
    spec             Spec
    engine           sim.Engine
    sim              *simv5.Simulation
}

// MakeBuilder returns a new Builder with default Spec.
func MakeBuilder() Builder {
    return Builder{spec: defaults()}
}

func (b Builder) WithEngine(engine sim.Engine) Builder { b.engine = engine; return b }
func (b Builder) WithSimulation(s *simv5.Simulation) Builder { b.sim = s; b.engine = s.Engine; return b }
func (b Builder) WithSpec(spec Spec) Builder           { b.spec = spec; return b }
func (b Builder) WithWidth(w int) Builder              { b.spec.Width = w; return b }
func (b Builder) WithLatency(cycles int) Builder       { b.spec.LatencyCycles = cycles; return b }
func (b Builder) WithFreq(freq sim.Freq) Builder       { b.spec.Freq = freq; return b }
func (b Builder) WithStorageRef(id string) Builder     { b.spec.StorageRef = id; return b }

// Build constructs the component. Ports are created but not connected.
func (b Builder) Build(name string) *Comp {
    _ = b.spec.validate()

    c := &Comp{Spec: b.spec}
    c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.spec.Freq, c)

    // Middlewares
    c.AddMiddleware(&ctrlMiddleware{Comp: c})
    // Pass emu registry and storage ref to middleware for resolution
    var emu *simv5.EmuStateRegistry
    if b.sim != nil { emu = b.sim.Emu() }
    c.AddMiddleware(&memMiddleware{Comp: c, emu: emu, storageRef: b.spec.StorageRef})

    // Initial state
    c.state = state{Mode: modeEnabled}

    return c
}
