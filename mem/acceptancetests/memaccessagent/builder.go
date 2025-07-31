// builder.go
package memaccessagent

import (
	"github.com/sarchlab/akita/v4/sim"
)

type Builder struct {
	name              string
	engine            sim.Engine
	freq              sim.Freq
	maxAddress        uint64
	writeLeft         int
	readLeft          int
	useVirtualAddress bool
	lowModule         sim.Port
}

func MakeBuilder() *Builder {
	return &Builder{
		name:       "MemAccessAgent",
		freq:       1 * sim.GHz,
		maxAddress: 1024 * 1024,
		writeLeft:  1000,
		readLeft:   1000,
	}
}

func (b *Builder) WithEngine(engine sim.Engine) *Builder {
	b.engine = engine
	return b
}

func (b *Builder) WithName(name string) *Builder {
	b.name = name
	return b
}

func (b *Builder) WithFreq(freq sim.Freq) *Builder {
	b.freq = freq
	return b
}

func (b *Builder) WithMaxAddress(addr uint64) *Builder {
	b.maxAddress = addr
	return b
}

func (b *Builder) WithWriteLeft(write int) *Builder {
	b.writeLeft = write
	return b
}

func (b *Builder) WithReadLeft(read int) *Builder {
	b.readLeft = read
	return b
}

func (b *Builder) UseVirtualAddress(use bool) *Builder {
	b.useVirtualAddress = use
	return b
}

func (b *Builder) WithLowModule(port sim.Port) *Builder {
	b.lowModule = port
	return b
}

func (b *Builder) Build(name string) *MemAccessAgent {
	agent := NewMemAccessAgent(b.engine)

	agent.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, agent)
	agent.MaxAddress = b.maxAddress
	agent.WriteLeft = b.writeLeft
	agent.ReadLeft = b.readLeft

	agent.UseVirtualAddress = b.useVirtualAddress

	agent.memPort = sim.NewPort(agent, 1, 1, name+".Mem")
	agent.AddPort("Mem", agent.memPort)

	if b.lowModule != nil {
		agent.LowModule = b.lowModule
	}

	return agent
}
