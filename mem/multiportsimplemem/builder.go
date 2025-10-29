package multiportsimplemem

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// Builder creates MultiPortSimpleMem components.
type Builder struct {
	engine           sim.Engine
	freq             sim.Freq
	latency          int
	concurrentSlots  int
	numPorts         int
	portBufferSize   int
	storage          *mem.Storage
	capacity         uint64
	addressConverter mem.AddressConverter
}

// MakeBuilder returns a Builder with sensible defaults.
func MakeBuilder() Builder {
	return Builder{
		freq:            1 * sim.GHz,
		latency:         10,
		concurrentSlots: 4,
		numPorts:        1,
		portBufferSize:  16,
		capacity:        4 * mem.GB,
	}
}

// WithEngine specifies the simulation engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq specifies the ticking frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithLatency specifies the service latency in cycles.
func (b Builder) WithLatency(latency int) Builder {
	b.latency = latency
	return b
}

// WithConcurrentSlots specifies how many requests can be served concurrently.
func (b Builder) WithConcurrentSlots(slots int) Builder {
	b.concurrentSlots = slots
	return b
}

// WithNumPorts specifies the number of top ports.
func (b Builder) WithNumPorts(num int) Builder {
	b.numPorts = num
	return b
}

// WithPortBufferSize specifies the depth of each port's buffers.
func (b Builder) WithPortBufferSize(size int) Builder {
	b.portBufferSize = size
	return b
}

// WithStorage injects an existing backing storage.
func (b Builder) WithStorage(storage *mem.Storage) Builder {
	b.storage = storage
	return b
}

// WithNewStorage creates a fresh storage with the given capacity.
func (b Builder) WithNewStorage(capacity uint64) Builder {
	b.capacity = capacity
	b.storage = nil
	return b
}

// WithAddressConverter specifies the address converter.
func (b Builder) WithAddressConverter(converter mem.AddressConverter) Builder {
	b.addressConverter = converter
	return b
}

// Build constructs a MultiPortSimpleMem component.
func (b Builder) Build(name string) *Comp {
	if b.concurrentSlots <= 0 {
		panic("multiportsimplemem: ConcurrentSlots must be > 0")
	}
	if b.numPorts <= 0 {
		panic("multiportsimplemem: NumPorts must be > 0")
	}
	if b.engine == nil {
		panic("multiportsimplemem: engine must be specified")
	}

	c := &Comp{
		Latency:          b.latency,
		ConcurrentSlots:  b.concurrentSlots,
		addressConverter: b.addressConverter,
		msgArrivalOrder:  make(map[string]uint64),
	}

	if b.storage != nil {
		c.Storage = b.storage
	} else {
		c.Storage = mem.NewStorage(b.capacity)
	}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)

	mw := &middleware{Comp: c}
	c.AddMiddleware(mw)

	c.topPorts = make([]sim.Port, 0, b.numPorts)
	for i := 0; i < b.numPorts; i++ {
		portName := fmt.Sprintf("%s.Top[%d]", name, i)
		port := sim.NewPort(c, b.portBufferSize, b.portBufferSize, portName)
		port.AcceptHook(&portArrivalHook{comp: c})
		c.AddPort(fmt.Sprintf("Top[%d]", i), port)
		c.topPorts = append(c.topPorts, port)
	}

	return c
}
