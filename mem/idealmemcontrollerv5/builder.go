package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

// Builder constructs a Comp either from a Spec or per-field setters.
type Builder struct {
    spec             Spec
    engine           sim.Engine
    storage          *mem.Storage
    addressConverter mem.AddressConverter
}

// MakeBuilder returns a new Builder with default Spec.
func MakeBuilder() Builder {
    return Builder{spec: Defaults()}
}

func (b Builder) WithEngine(engine sim.Engine) Builder { b.engine = engine; return b }
func (b Builder) WithSpec(spec Spec) Builder           { b.spec = spec; return b }
func (b Builder) WithWidth(w int) Builder              { b.spec.Width = w; return b }
func (b Builder) WithLatency(cycles int) Builder       { b.spec.LatencyCycles = cycles; return b }
func (b Builder) WithFreq(freq sim.Freq) Builder       { b.spec.Freq = freq; return b }
func (b Builder) WithTopBufSize(n int) Builder         { b.spec.TopBufSize = n; return b }
func (b Builder) WithCtrlBufSize(n int) Builder        { b.spec.CtrlBufSize = n; return b }
func (b Builder) WithNewStorage(cap uint64) Builder    { b.storage = nil; b.spec.CapacityBytes = cap; return b }
func (b Builder) WithStorage(storage *mem.Storage) Builder { b.storage = storage; return b }
func (b Builder) WithUnitSize(unit uint64) Builder     { b.spec.UnitSize = unit; return b }
func (b Builder) WithAddressConverter(ac mem.AddressConverter) Builder {
    b.addressConverter = ac
    return b
}

// Build constructs the component. Ports are created but not connected.
func (b Builder) Build(name string) *Comp {
    _ = b.spec.Validate()

    c := &Comp{Spec: b.spec}
    c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.spec.Freq, c)

    // Storage wiring
    if b.storage != nil {
        c.Storage = b.storage
    } else {
        if b.spec.UnitSize == 0 {
            c.Storage = mem.NewStorage(b.spec.CapacityBytes)
        } else {
            c.Storage = mem.NewStorageWithUnitSize(b.spec.CapacityBytes, b.spec.UnitSize)
        }
    }
    c.AddressConverter = b.addressConverter

    // Middlewares
    c.AddMiddleware(&ctrlMiddleware{Comp: c})
    c.AddMiddleware(&memMiddleware{Comp: c})

    // Ports
    c.IO.Top = sim.NewPort(c, b.spec.TopBufSize, b.spec.TopBufSize, name+".Top")
    c.AddPort("Top", c.IO.Top)
    c.IO.Control = sim.NewPort(c, b.spec.CtrlBufSize, b.spec.CtrlBufSize, name+".Control")
    c.AddPort("Control", c.IO.Control)

    // Initial state
    c.State = State{Mode: ModeEnabled}

    return c
}
