package mmuCache

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder can build mmuCache
type Builder struct {
	engine          sim.Engine
	freq            sim.Freq
	numReqPerCycle  int
	numLevels       int
	numBlocks       int
	pageSize        uint64
	lowModule       sim.RemotePort
	upModule        sim.RemotePort
	latencyPerLevel uint64
	log2PageSize    uint64

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		freq:            1 * sim.GHz,
		numReqPerCycle:  4,
		numLevels:       5,
		numBlocks:       1,
		pageSize:        4096,
		latencyPerLevel: 100,
		log2PageSize:    12,
	}
}

// WithLatencyPerLevel sets the latency per level
func (b Builder) WithLatencyPerLevel(latency uint64) Builder {
	b.latencyPerLevel = latency
	return b
}

// WithUpperModule sets the upper module remote port
func (b Builder) WithUpperModule(m sim.RemotePort) Builder {
	b.upModule = m
	return b
}

// WithNumLevels sets the number of levels in the mmuCache
func (b Builder) WithNumLevels(n int) Builder {
	b.numLevels = n
	return b
}

// WithEngine sets the engine that the mmuCache to use
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the freq the mmuCache use
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumBlocks sets the number of blocks in a mmuCache.
func (b Builder) WithNumBlocks(n int) Builder {
	b.numBlocks = n
	return b
}

// WithPageSize sets the page size that the mmuCache works with.
func (b Builder) WithPageSize(n uint64) Builder {
	b.pageSize = n
	return b
}

// WithNumReqPerCycle sets the number of requests per cycle can be processed by
// a mmuCache
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithLowModule sets the port that can provide the address translation in case
// of mmuCache miss.
func (b Builder) WithLowModule(lowModule sim.RemotePort) Builder {
	b.lowModule = lowModule
	return b
}

// WithTopPort sets the top port for the mmuCache
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithBottomPort sets the bottom port for the mmuCache
func (b Builder) WithBottomPort(port sim.Port) Builder {
	b.bottomPort = port
	return b
}

// WithControlPort sets the control port for the mmuCache
func (b Builder) WithControlPort(port sim.Port) Builder {
	b.controlPort = port
	return b
}

// Build creates a new mmuCache
func (b Builder) Build(name string) *Comp {
	if b.numBlocks <= 0 {
		panic("mmuCache.Builder: numBlocks must be > 0")
	}

	spec := Spec{
		NumBlocks:       b.numBlocks,
		NumLevels:       b.numLevels,
		PageSize:        b.pageSize,
		Log2PageSize:    b.log2PageSize,
		NumReqPerCycle:  b.numReqPerCycle,
		LatencyPerLevel: b.latencyPerLevel,
		LowModulePort:   b.lowModule,
		UpModulePort:    b.upModule,
	}

	initialState := State{
		CurrentState: mmuCacheStateEnable,
		Table:        initSets(b.numLevels, b.numBlocks),
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	c := &Comp{
		Component: modelComp,
	}

	b.topPort.SetComponent(c)
	modelComp.AddPort("Top", b.topPort)

	b.bottomPort.SetComponent(c)
	modelComp.AddPort("Bottom", b.bottomPort)

	b.controlPort.SetComponent(c)
	modelComp.AddPort("Control", b.controlPort)

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	cacheMW := &mmuCacheMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cacheMW)

	return c
}
