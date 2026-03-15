package mmuCache

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides the default configuration for mmuCache components.
var DefaultSpec = Spec{
	Freq:            1 * sim.GHz,
	NumReqPerCycle:  4,
	NumLevels:       5,
	NumBlocks:       1,
	PageSize:        4096,
	LatencyPerLevel: 100,
	Log2PageSize:    12,
}

// A Builder can build mmuCache
type Builder struct {
	engine sim.EventScheduler
	spec   Spec

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithLatencyPerLevel sets the latency per level
func (b Builder) WithLatencyPerLevel(latency uint64) Builder {
	b.spec.LatencyPerLevel = latency
	return b
}

// WithUpperModule sets the upper module remote port
func (b Builder) WithUpperModule(m sim.RemotePort) Builder {
	b.spec.UpModulePort = m
	return b
}

// WithNumLevels sets the number of levels in the mmuCache
func (b Builder) WithNumLevels(n int) Builder {
	b.spec.NumLevels = n
	return b
}

// WithEngine sets the engine that the mmuCache to use
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the freq the mmuCache use
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithNumBlocks sets the number of blocks in a mmuCache.
func (b Builder) WithNumBlocks(n int) Builder {
	b.spec.NumBlocks = n
	return b
}

// WithPageSize sets the page size that the mmuCache works with.
func (b Builder) WithPageSize(n uint64) Builder {
	b.spec.PageSize = n
	return b
}

// WithNumReqPerCycle sets the number of requests per cycle can be processed by
// a mmuCache
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.spec.NumReqPerCycle = n
	return b
}

// WithLowModule sets the port that can provide the address translation in case
// of mmuCache miss.
func (b Builder) WithLowModule(lowModule sim.RemotePort) Builder {
	b.spec.LowModulePort = lowModule
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
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	if b.spec.NumBlocks <= 0 {
		panic("mmuCache.Builder: numBlocks must be > 0")
	}

	spec := b.spec

	initialState := State{
		CurrentState: mmuCacheStateEnable,
		Table:        initSets(b.spec.NumLevels, b.spec.NumBlocks),
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	b.topPort.SetComponent(modelComp)
	modelComp.AddPort("Top", b.topPort)

	b.bottomPort.SetComponent(modelComp)
	modelComp.AddPort("Bottom", b.bottomPort)

	b.controlPort.SetComponent(modelComp)
	modelComp.AddPort("Control", b.controlPort)

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	cacheMW := &mmuCacheMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cacheMW)

	return modelComp
}
