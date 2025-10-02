package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/mem/mem"
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

func (b Builder) WithSimulation(s *simv5.Simulation) Builder { b.sim = s; b.engine = s.GetEngine(); return b }
func (b Builder) WithSpec(spec Spec) Builder           { b.spec = spec; return b }

// Build constructs the component. Ports are created but not connected.
func (b Builder) Build(name string) *Comp {
    _ = b.spec.validate()

    if b.engine == nil {
        panic("idealmemcontrollerv5.Builder: engine is nil; call WithSimulation")
    }

    c := &Comp{Spec: b.spec}
    c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.spec.Freq, c)

    // Resolve address converter locally based on spec (no global registry)
    var conv AddressConverter
    switch b.spec.AddrConv.Kind {
    case "", "identity":
        conv = nil
    case "interleaving":
        var c mem.InterleavingConverter
        params := b.spec.AddrConv.Params
        c.InterleavingSize = params["InterleavingSize"]
        c.TotalNumOfElements = int(params["TotalNumOfElements"])
        c.CurrentElementIndex = int(params["CurrentElementIndex"])
        c.Offset = params["Offset"]
        conv = c
    default:
        // Unknown kind; leave conv nil (identity)
        conv = nil
    }

    // Middlewares
    c.AddMiddleware(&ctrlMiddleware{Comp: c})
    c.AddMiddleware(&memMiddleware{Comp: c, sim: b.sim, storageRef: b.spec.StorageRef, conv: conv})

    // Initial state
    c.state = state{Mode: modeEnabled}

    return c
}
