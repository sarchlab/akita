// builder.go
package memaccessagent

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Builder constructs MemAccessAgent instances.
type Builder struct {
	spec      Spec
	writeLeft int
	readLeft  int
	engine    timing.EventScheduler
	lowModule messaging.Port
	memPort   messaging.Port
}

// MakeBuilder returns a new Builder with sensible defaults.
func MakeBuilder() *Builder {
	return &Builder{
		spec: Spec{
			Freq:       1 * timing.GHz,
			MaxAddress: 1024 * 1024,
		},
		writeLeft: 1000,
		readLeft:  1000,
	}
}

// WithEngine sets the simulation engine.
func (b *Builder) WithEngine(engine timing.EventScheduler) *Builder {
	b.engine = engine
	return b
}

// WithName is a no-op kept for backward compatibility; name is passed to Build.
func (b *Builder) WithName(_ string) *Builder {
	return b
}

// WithFreq sets the tick frequency.
func (b *Builder) WithFreq(freq timing.Freq) *Builder {
	b.spec.Freq = freq
	return b
}

// WithMaxAddress sets the address space size.
func (b *Builder) WithMaxAddress(addr uint64) *Builder {
	b.spec.MaxAddress = addr
	return b
}

// WithWriteLeft sets the initial number of writes to perform.
func (b *Builder) WithWriteLeft(write int) *Builder {
	b.writeLeft = write
	return b
}

// WithReadLeft sets the initial number of reads to perform.
func (b *Builder) WithReadLeft(read int) *Builder {
	b.readLeft = read
	return b
}

// UseVirtualAddress configures whether virtual addresses are used.
func (b *Builder) UseVirtualAddress(use bool) *Builder {
	b.spec.UseVirtualAddress = use
	return b
}

// WithLowModule sets the downstream module port.
func (b *Builder) WithLowModule(port messaging.Port) *Builder {
	b.lowModule = port
	return b
}

// WithMemPort sets the port used to send/receive memory messages.
func (b *Builder) WithMemPort(port messaging.Port) *Builder {
	b.memPort = port
	return b
}

// Build creates a new MemAccessAgent with the given name.
func (b *Builder) Build(name string) *MemAccessAgent {
	initialState := State{
		WriteLeft:       b.writeLeft,
		ReadLeft:        b.readLeft,
		KnownMemValue:   make(map[uint64][]uint32),
		PendingReadReq:  make(map[uint64]mem.ReadReq),
		PendingWriteReq: make(map[uint64]mem.WriteReq),
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)
	modelComp.State = initialState

	agent := &MemAccessAgent{
		Component: modelComp,
	}

	if b.lowModule != nil {
		agent.LowModule = b.lowModule
	}

	mw := &agentMiddleware{agent: agent}
	modelComp.AddMiddleware(mw)

	b.memPort.SetComponent(agent)
	modelComp.AddPort("Mem", b.memPort)

	return agent
}
