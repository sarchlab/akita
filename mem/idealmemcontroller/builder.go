package idealmemcontroller

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type Builder struct {
	width            int
	latency          int
	freq             timing.Freq
	capacity         uint64
	simulation       simulation.Simulation
	cacheLineSize    int
	topBufSize       int
	storage          *mem.Storage
	addressConverter mem.AddressConverter
}

// MakeBuilder returns a new Builder
func MakeBuilder() Builder {
	return Builder{
		latency:       100,
		freq:          1 * timing.GHz,
		capacity:      4 * mem.GB,
		cacheLineSize: 64,
		width:         1,
		topBufSize:    16,
	}
}

// WithWidth sets the width of the memory controller
func (b Builder) WithWidth(width int) Builder {
	b.width = width
	return b
}

// WithLatency sets the latency of the memory controller
func (b Builder) WithLatency(latency int) Builder {
	b.latency = latency
	return b
}

// WithFreq sets the frequency of the memory controller
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithNewStorage sets the capacity of the memory controller
func (b Builder) WithNewStorage(capacity uint64) Builder {
	b.capacity = capacity
	return b
}

// WithCacheLineSize sets the cache line size of the memory controller
func (b Builder) WithCacheLineSize(cacheLineSize int) Builder {
	b.cacheLineSize = cacheLineSize
	return b
}

// WithSimulation sets the simulation of the memory controller
func (b Builder) WithSimulation(simulation simulation.Simulation) Builder {
	b.simulation = simulation
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

// Build builds a new Comp
func (b Builder) Build(
	name string,
) *Comp {
	c := &Comp{
		Latency: b.latency,
		width:   b.width,
	}

	c.TickingComponent = modeling.NewTickingComponent(
		name, b.simulation.GetEngine(), b.freq, c)
	c.Latency = b.latency
	c.addressConverter = b.addressConverter

	if b.storage == nil {
		c.Storage = mem.NewStorage(b.capacity)
	} else {
		c.Storage = b.storage
	}

	c.topPort = modeling.PortBuilder{}.
		WithSimulation(b.simulation).
		WithComponent(c).
		WithIncomingBufCap(b.topBufSize).
		WithOutgoingBufCap(b.topBufSize).
		Build(name + ".TopPort")
	c.AddPort("Top", c.topPort)

	middleware := &middleware{Comp: c}
	c.AddMiddleware(middleware)

	return c
}
