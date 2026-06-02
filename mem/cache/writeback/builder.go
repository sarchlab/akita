package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides default configuration for the writeback cache.
var defaultSpec = Spec{
	Freq:                  1 * timing.GHz,
	NumReqPerCycle:        1,
	Log2BlockSize:         6,
	BankLatency:           10,
	WayAssociativity:      4,
	NumBanks:              1,
	NumMSHREntry:          16,
	TotalByteSize:         512 * mem.KB,
	WriteBufferCapacity:   1024,
	MaxInflightFetch:      128,
	MaxInflightEviction:   128,
	InterleavingSize:      4096,
	TopPortBufferSize:     8,
	BottomPortBufferSize:  8,
	ControlPortBufferSize: 8,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder can build writeback caches. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithRegistrar and WithResources.
// The component creates its own ports.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
}

// MakeBuilder creates a new builder with default configurations.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
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

// Build creates a usable writeback cache. It creates the component's Top,
// Bottom, and Control ports.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("writeback: WithRegistrar is required")
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
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(Resources{
			Storage:             storage,
			AddressToPortMapper: b.resources.AddressToPortMapper,
			RemotePorts:         b.resources.RemotePorts,
		}).
		Build(name)

	comp.State = initialState

	pmw := b.buildPipelineMW(comp, name, spec, laneWidth)
	cmw := b.buildControlMW(comp, name, spec, pmw)

	comp.AddMiddleware(pmw) // index 0
	comp.AddMiddleware(cmw) // index 1

	b.registrar.RegisterComponent(comp)

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
		WithSimulation(b.registrar).
		Build(name + ".Storage")
}

func (b Builder) buildInitialState(
	name string, spec Spec, laneWidth, numSets int,
) State {
	blockSize := 1 << spec.Log2BlockSize

	s := State{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf: queueing.NewBuffer[int](
			name+".DirStageBuf", spec.NumReqPerCycle),
		DirToBankBufs: []queueing.Buffer[int]{
			queueing.NewBuffer[int](name+".DirToBankBuf", spec.NumReqPerCycle),
		},
		WriteBufferToBankBufs: []queueing.Buffer[int]{
			queueing.NewBuffer[int](
				name+".WriteBufferToBankBuf", spec.NumReqPerCycle),
		},
		MSHRStageBuf: queueing.NewBuffer[int](
			name+".MSHRStageBuf", spec.NumReqPerCycle),
		WriteBufferBuf: queueing.NewBuffer[int](
			name+".WriteBufferBuf", spec.NumReqPerCycle),
		DirPipeline: queueing.NewPipeline[int](laneWidth, spec.DirLatency),
		DirPostPipelineBuf: queueing.NewBuffer[int](
			name+".DirPostPipelineBuf", spec.NumReqPerCycle),
		BankPipelines: []queueing.Pipeline[int]{
			queueing.NewPipeline[int](laneWidth, spec.BankLatency),
		},
		BankPostPipelineBufs: []postPipelineBuf{
			newPostPipelineBuf(laneWidth),
		},
		BankInflightTransCounts:         make([]int, 1),
		BankDownwardInflightTransCounts: make([]int, 1),
	}

	cache.DirectoryReset(
		&s.DirectoryState, numSets, spec.WayAssociativity, blockSize)

	return s
}

// buildSpec produces the final Spec used by the component. It derives the
// number of sets and resolves the address mapper (from an injected mapper or
// from the type string plus the remote ports in Resources) into the flat
// address-mapping fields read at Tick time.
func (b Builder) buildSpec(numSets int) Spec {
	spec := b.spec
	spec.NumBanks = 1
	spec.NumSets = numSets

	mapperType, remotePorts, interleavingSize := b.resolveAddressMapper()
	if mapperType != "" {
		remotePortNames := make([]string, len(remotePorts))
		for i, rp := range remotePorts {
			remotePortNames[i] = string(rp)
		}
		spec.AddressMapperType = mapperType
		spec.RemotePortNames = remotePortNames
		spec.InterleavingSize = interleavingSize
	}

	return spec
}

// resolveAddressMapper returns the address mapper type, remote ports, and
// interleaving size. An externally injected mapper (Resources.AddressToPortMapper)
// takes precedence and is decomposed into these fields; otherwise the values
// come from Spec.AddressMapperType plus Resources.RemotePorts.
func (b Builder) resolveAddressMapper() (
	mapperType string,
	remotePorts []messaging.RemotePort,
	interleavingSize uint64,
) {
	if b.resources.AddressToPortMapper != nil {
		switch m := b.resources.AddressToPortMapper.(type) {
		case *mem.SinglePortMapper:
			return "single", []messaging.RemotePort{m.Port}, b.spec.InterleavingSize
		case *mem.InterleavedAddressPortMapper:
			return "interleaved", m.LowModules, m.InterleavingSize
		default:
			panic(fmt.Sprintf(
				"unsupported address mapper type: %T",
				b.resources.AddressToPortMapper))
		}
	}

	return b.spec.AddressMapperType, b.resources.RemotePorts, b.spec.InterleavingSize
}

func (b Builder) buildPipelineMW(
	comp *modeling.Component[Spec, State, Resources],
	name string,
	spec Spec,
	laneWidth int,
) *pipelineMW {
	m := &pipelineMW{
		comp: comp,
	}

	b.createPipelinePorts(m, comp, name, spec)

	m.storage = comp.Resources().Storage

	b.createInternalStages(m, laneWidth)

	return m
}

func (b Builder) buildControlMW(
	comp *modeling.Component[Spec, State, Resources],
	name string,
	spec Spec,
	pmw *pipelineMW,
) *controlMW {
	controlPort := messaging.NewPort(
		comp, spec.ControlPortBufferSize, spec.ControlPortBufferSize,
		name+".Control")
	comp.AddPort("Control", controlPort)

	f := &flusher{
		pipeline: pmw,
		ctrlPort: controlPort,
	}

	cmw := &controlMW{
		comp:    comp,
		flusher: f,
	}

	return cmw
}

func (b Builder) createPipelinePorts(
	m *pipelineMW,
	comp *modeling.Component[Spec, State, Resources],
	name string,
	spec Spec,
) {
	m.topPort = messaging.NewPort(
		comp, spec.TopPortBufferSize, spec.TopPortBufferSize, name+".Top")
	comp.AddPort("Top", m.topPort)

	m.bottomPort = messaging.NewPort(
		comp, spec.BottomPortBufferSize, spec.BottomPortBufferSize,
		name+".Bottom")
	comp.AddPort("Bottom", m.bottomPort)
}

func (b Builder) createInternalStages(m *pipelineMW, laneWidth int) {
	m.topParser = &topParser{cache: m}

	m.dirStage = &directoryStage{
		cache: m,
	}

	m.bankStages = make([]*bankStage, 1)
	m.bankStages[0] = &bankStage{
		cache:         m,
		bankID:        0,
		pipelineWidth: laneWidth,
	}

	m.mshrStage = &mshrStage{cache: m}
	m.writeBuffer = &writeBufferStage{
		cache: m,
	}
}
