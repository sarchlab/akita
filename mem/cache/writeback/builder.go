package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// A Builder can build writeback caches. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithSimulation and WithResources.
// The component declares its "Top", "Bottom", and "Control" ports; the port
// instances are supplied externally after Build with AssignPort (the caller
// chooses the buffer sizes).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
	resources  Resources
}

// MakeBuilder creates a new builder with default configurations.
func MakeBuilder() Builder {
	return Builder{spec: Definition.DefaultSpec}
}

// WithSimulation sets the simulation that owns and registers the built component.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from Definition.DefaultSpec and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the component's shared resources and wiring (e.g. a
// storage shared with other components, the address-to-port mapper, and the
// remote ports). If Storage is not set, the component builds its own.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build creates a usable writeback cache. It declares the component's "Top",
// "Bottom", and "Control" ports; assign the port instances after Build with
// AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("writeback: WithSimulation is required")
	}

	blockSize := 1 << b.spec.Log2BlockSize
	numSets := int(
		b.spec.TotalByteSize / uint64(b.spec.WayAssociativity*blockSize))

	spec := b.buildSpec(numSets)

	laneWidth := spec.NumReqPerCycle
	if laneWidth == 1 {
		laneWidth = 2
	}

	initialState := b.buildInitialState(name, spec, laneWidth, numSets)

	storage := b.resolveStorage(name, spec)

	comp := modeling.NewBuilder[Spec, State, Resources]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithDefinition(Definition).
		WithResources(Resources{
			Storage:             storage,
			AddressToPortMapper: b.resolveAddressMapper(),
			RemotePorts:         b.resources.RemotePorts,
		}).
		Build(name)

	comp.State = initialState

	pmw := b.buildPipelineMW(comp, laneWidth)
	cmw := b.buildControlMW(comp, pmw)
	ucmw := &ctrlMiddleware{pipeline: pmw}

	// Control runs before the data pipeline so a Pause/Drain/Reset takes
	// effect this tick before any Top/Bottom traffic or in-flight operation
	// advances. The flusher (legacy control) then runs, and the data pipeline
	// last.
	comp.AddMiddleware(ucmw) // index 0: universal control verbs
	comp.AddMiddleware(cmw)  // index 1: legacy flush walker
	comp.AddMiddleware(pmw)  // index 2: data pipeline

	b.simulation.RegisterComponent(comp)

	return comp
}

// resolveStorage returns the injected storage, or builds a default one sized by
// Spec.TotalByteSize.
func (b Builder) resolveStorage(name string, spec Spec) *mem.Storage {
	if b.resources.Storage != nil {
		return b.resources.Storage
	}

	return mem.MakeStorageBuilder().
		WithCapacity(spec.TotalByteSize).
		WithSimulation(b.simulation).
		Build(name + ".Storage")
}

func (b Builder) buildInitialState(
	name string, spec Spec, laneWidth, numSets int,
) State {
	blockSize := 1 << spec.Log2BlockSize
	numBanks := spec.NumBanks

	dirToBank := make([]queueing.Buffer[int], numBanks)
	wbToBank := make([]queueing.Buffer[int], numBanks)
	bankPipes := make([]queueing.Pipeline[int], numBanks)
	bankPostBufs := make([]queueing.Buffer[int], numBanks)
	for i := 0; i < numBanks; i++ {
		dirToBank[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.DirToBankBuf%d", name, i), spec.NumReqPerCycle)
		wbToBank[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.WriteBufferToBankBuf%d", name, i),
			spec.NumReqPerCycle)
		bankPipes[i] = queueing.NewPipeline[int](laneWidth, spec.BankLatency)
		bankPostBufs[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.BankPostPipelineBuf%d", name, i), laneWidth)
	}

	s := State{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf: queueing.NewBuffer[int](
			name+".DirStageBuf", spec.NumReqPerCycle),
		DirToBankBufs:         dirToBank,
		WriteBufferToBankBufs: wbToBank,
		MSHRStageBuf: queueing.NewBuffer[int](
			name+".MSHRStageBuf", spec.NumReqPerCycle),
		WriteBufferBuf: queueing.NewBuffer[int](
			name+".WriteBufferBuf", spec.NumReqPerCycle),
		DirPipeline: queueing.NewPipeline[int](laneWidth, spec.DirLatency),
		DirPostPipelineBuf: queueing.NewBuffer[int](
			name+".DirPostPipelineBuf", spec.NumReqPerCycle),
		BankPipelines:                   bankPipes,
		BankPostPipelineBufs:            bankPostBufs,
		BankInflightTransCounts:         make([]int, numBanks),
		BankDownwardInflightTransCounts: make([]int, numBanks),
	}

	cache.DirectoryReset(
		&s.DirectoryState, numSets, spec.WayAssociativity, blockSize)

	return s
}

// buildSpec produces the final Spec used by the component: it derives the
// number of sets and defaults the bank count.
func (b Builder) buildSpec(numSets int) Spec {
	spec := b.spec
	if spec.NumBanks < 1 {
		spec.NumBanks = 1
	}
	spec.NumSets = numSets

	return spec
}

// resolveAddressMapper returns the mapper that routes fetches and evictions
// to lower memory: the injected Resources.AddressToPortMapper, or one built
// from Spec.AddressMapperType over Resources.RemotePorts. It returns nil when
// neither is configured or there are no remote ports, so the first routed
// request reports the missing wiring.
func (b Builder) resolveAddressMapper() mem.AddressToPortMapper {
	if b.resources.AddressToPortMapper != nil {
		return b.resources.AddressToPortMapper
	}

	ports := b.resources.RemotePorts

	switch b.spec.AddressMapperType {
	case "":
		return nil
	case "single":
		if len(ports) == 0 {
			return nil
		}

		return &mem.SinglePortMapper{Port: ports[0]}
	case "interleaved":
		if len(ports) == 0 {
			return nil
		}

		if b.spec.InterleavingSize == 0 {
			panic("writeback: an interleaved address mapper needs a " +
				"non-zero Spec.InterleavingSize")
		}

		mapper := mem.NewInterleavedAddressPortMapper(b.spec.InterleavingSize)
		mapper.LowModules = append(mapper.LowModules, ports...)

		return mapper
	default:
		panic(fmt.Sprintf(
			"writeback: unknown address mapper type %q",
			b.spec.AddressMapperType))
	}
}

func (b Builder) buildPipelineMW(
	comp *modeling.Component[Spec, State, Resources],
	laneWidth int,
) *pipelineMW {
	m := &pipelineMW{
		comp: comp,
	}

	m.storage = comp.Resources().Storage

	b.createInternalStages(m, laneWidth)

	return m
}

func (b Builder) buildControlMW(
	comp *modeling.Component[Spec, State, Resources],
	pmw *pipelineMW,
) *controlMW {
	f := &flusher{
		pipeline: pmw,
	}

	cmw := &controlMW{
		comp:    comp,
		flusher: f,
	}

	return cmw
}

func (b Builder) createInternalStages(m *pipelineMW, laneWidth int) {
	m.topParser = &topParser{cache: m}

	m.dirStage = &directoryStage{
		cache: m,
	}

	numBanks := m.comp.Spec().NumBanks
	m.bankStages = make([]*bankStage, numBanks)
	for i := 0; i < numBanks; i++ {
		m.bankStages[i] = &bankStage{
			cache:         m,
			bankID:        i,
			pipelineWidth: laneWidth,
		}
	}

	m.mshrStage = &mshrStage{cache: m}
	m.writeBuffer = &writeBufferStage{
		cache: m,
	}
}
