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
func (b Builder) WithSimulation(s *simv5.Simulation) Builder { b.sim = s; b.engine = s.GetEngine(); return b }
func (b Builder) WithSpec(spec Spec) Builder           { b.spec = spec; return b }
func (b Builder) WithWidth(w int) Builder              { b.spec.Width = w; return b }
func (b Builder) WithLatency(cycles int) Builder       { b.spec.LatencyCycles = cycles; return b }
func (b Builder) WithFreq(freq sim.Freq) Builder       { b.spec.Freq = freq; return b }
func (b Builder) WithStorageRef(id string) Builder     { b.spec.StorageRef = id; return b }
// Address converter configuration (primitive-only)
func (b Builder) WithIdentityAddressing() Builder {
    b.spec.AddrConv = AddressConvSpec{Kind: "identity", Params: map[string]uint64{}}
    return b
}
func (b Builder) WithInterleaving(blockSize uint64, total, index int, offset uint64) Builder {
    b.spec.AddrConv = AddressConvSpec{Kind: "interleaving", Params: map[string]uint64{
        "InterleavingSize":    blockSize,
        "TotalNumOfElements":  uint64(total),
        "CurrentElementIndex": uint64(index),
        "Offset":              offset,
    }}
    return b
}
func (b Builder) WithAddressConverter(kind string, params map[string]uint64) Builder {
    // Defensive copy
    cp := make(map[string]uint64, len(params))
    for k, v := range params { cp[k] = v }
    b.spec.AddrConv = AddressConvSpec{Kind: kind, Params: cp}
    return b
}

// Build constructs the component. Ports are created but not connected.
func (b Builder) Build(name string) *Comp {
    _ = b.spec.validate()

    c := &Comp{Spec: b.spec}
    c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.spec.Freq, c)

    // Resolve address converter via sim registry (with built-ins as fallback)
    var conv interface{}
    if b.sim != nil {
        if ac, err := b.sim.BuildAddressConverter(b.spec.AddrConv.Kind, b.spec.AddrConv.Params); err == nil {
            conv = ac
        }
    }

    // Middlewares
    c.AddMiddleware(&ctrlMiddleware{Comp: c})
    c.AddMiddleware(&memMiddleware{Comp: c, sim: b.sim, storageRef: b.spec.StorageRef, conv: conv})

    // Initial state
    c.state = state{Mode: modeEnabled}

    return c
}
