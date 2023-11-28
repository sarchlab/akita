package idealmemcontroller

import (
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
)

type Builder struct {
	width            int
	latency          int
	freq             sim.Freq
	capacity         uint64
	engine           sim.Engine
	cacheLineSize    int
	topBufSize       int
	storage          *mem.Storage
	addressConverter mem.AddressConverter
}

// MakeBuilder returns a new Builder
func MakeBuilder() Builder {
	return Builder{
		latency:       100,
		freq:          1 * sim.GHz,
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
func (b Builder) WithFreq(freq sim.Freq) Builder {
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
func (b Builder) WithAddressConverter(addressConverter mem.AddressConverter) Builder {
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

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)
	c.Latency = b.latency
	c.addressConverter = b.addressConverter

	if b.storage == nil {
		c.Storage = mem.NewStorage(b.capacity)
	} else {
		c.Storage = b.storage
	}

	c.topPort = sim.NewLimitNumMsgPort(c, b.topBufSize, name+".TopPort")
	c.AddPort("Top", c.topPort)

	return c
}
