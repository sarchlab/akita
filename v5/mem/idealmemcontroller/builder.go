package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// Builder builds ideal memory controller components.
type Builder struct {
	spec             *Spec
	freq             sim.Freq
	capacity         uint64
	engine           sim.Engine
	topBufSize       int
	storage          *mem.Storage
	addressConverter mem.AddressConverter
	topPort          sim.Port
	ctrlPort         sim.Port
}

// MakeBuilder returns a new Builder
func MakeBuilder() Builder {
	return Builder{
		freq:       1 * sim.GHz,
		capacity:   4 * mem.GB,
		topBufSize: 16,
		spec: &Spec{
			Latency:       100,
			Width:         1,
			CacheLineSize: 64,
		},
	}
}

// WithSpec sets the spec of the memory controller
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = &spec
	return b
}

// WithFreq sets the frequency of the memory controller
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNewStorage sets the capacity of the memory controller
func (b Builder) WithNewStorage(capacity uint64) Builder {
	b.capacity = capacity
	return b
}

// WithEngine sets the engine of the memory controller
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithTopBufSize sets the size of the top buffer
func (b Builder) WithTopBufSize(topBufSize int) Builder {
	b.topBufSize = topBufSize
	return b
}

// WithStorage sets the storage of the memory controller
func (b Builder) WithStorage(storage *mem.Storage) Builder {
	b.storage = storage
	return b
}

// WithAddressConverter sets the address converter of the memory controller
func (b Builder) WithAddressConverter(
	addressConverter mem.AddressConverter,
) Builder {
	b.addressConverter = addressConverter
	return b
}

// WithTopPort sets the top port of the memory controller
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithCtrlPort sets the control port of the memory controller
func (b Builder) WithCtrlPort(port sim.Port) Builder {
	b.ctrlPort = port
	return b
}

// Build builds a new Comp
func (b Builder) Build(
	name string,
) *Comp {
	spec := *b.spec
	spec.StorageRef = name

	initialState := State{
		CurrentState: "enable",
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	c := &Comp{
		Component:        modelComp,
		addressConverter: b.addressConverter,
	}

	if b.storage == nil {
		c.Storage = mem.NewStorage(b.capacity)
	} else {
		c.Storage = b.storage
	}

	ctrlMiddleware := &ctrlMiddleware{Comp: c}
	c.AddMiddleware(ctrlMiddleware)
	funcMiddleware := &memMiddleware{Comp: c}
	c.AddMiddleware(funcMiddleware)

	c.topPort = b.topPort
	c.topPort.SetComponent(c)
	c.AddPort("Top", c.topPort)
	c.ctrlPort = b.ctrlPort
	c.ctrlPort.SetComponent(c)
	c.AddPort("Control", c.ctrlPort)

	return c
}
