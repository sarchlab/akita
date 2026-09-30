package writethroughcache

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// A Builder can build a writethroughcache cache. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithSimulation and
// WithResources. The component declares its "Top", "Bottom", and "Control"
// ports; the port instances are supplied externally after Build with AssignPort
// (the caller chooses the buffer sizes).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
	resources  Resources
}

// MakeBuilder creates a builder with default parameter setting.
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

// WithResources injects the component's shared resources and external wiring
// (storage, the address-to-port mapper, and the remote ports the cache sends
// requests to). If not set, the component builds its own storage.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build returns a new cache component. It declares the component's "Top",
// "Bottom", and "Control" ports; assign the port instances after Build with
// AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("writethroughcache: WithSimulation is required")
	}

	spec := b.spec
	if spec.WritePolicyType == "" {
		spec.WritePolicyType = "write-around"
	}

	blockSize := 1 << spec.Log2BlockSize
	spec.NumSets = int(spec.TotalByteSize /
		uint64(spec.WayAssociativity*blockSize))

	initialState := b.buildInitialState(name, spec, spec.NumSets, blockSize)

	storage := b.resolveStorage(name, spec)

	comp := modeling.NewBuilder[Spec, State, Resources]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithDefinition(Definition).
		WithResources(Resources{
			Storage:       storage,
			AddressMapper: b.resolveAddressMapper(),
		}).
		Build(name)

	comp.State = initialState

	pmw := b.buildPipelineMW(comp)
	b.buildStages(pmw, spec)

	ucmw := &ctrlMiddleware{pipeline: pmw}

	// Control runs before the data pipeline so a Pause/Drain/Reset takes
	// effect this tick before any Top/Bottom traffic advances.
	comp.AddMiddleware(ucmw) // index 0: control verbs
	comp.AddMiddleware(pmw)  // index 1: data pipeline

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

// resolveAddressMapper returns the mapper that routes requests to lower
// memory: the injected Resources.AddressMapper, or one built from
// Spec.AddressMapperType over Resources.RemotePorts. It returns nil when
// neither is configured or there are no remote ports, so the first routed
// request reports the missing wiring.
func (b Builder) resolveAddressMapper() mem.AddressToPortMapper {
	if b.resources.AddressMapper != nil {
		return b.resources.AddressMapper
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
			panic("writethroughcache: an interleaved address mapper needs a " +
				"non-zero Spec.InterleavingSize")
		}

		mapper := mem.NewInterleavedAddressPortMapper(b.spec.InterleavingSize)
		mapper.LowModules = append(mapper.LowModules, ports...)

		return mapper
	default:
		panic(fmt.Sprintf(
			"writethroughcache: unknown address mapper type %q",
			b.spec.AddressMapperType))
	}
}

func (b *Builder) buildInitialState(
	name string,
	spec Spec,
	numSets, blockSize int,
) State {
	bankBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankBufs[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.Bank%d.Buffer", name, i),
			spec.NumReqPerCycle,
		)
	}

	bankPipelines := make([]queueing.Pipeline[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPipelines[i] = queueing.NewPipeline[int](
			spec.NumReqPerCycle,
			spec.BankLatency,
		)
	}

	bankPostBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPostBufs[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", name, i),
			spec.NumReqPerCycle,
		)
	}

	initialState := State{
		DirBuf: queueing.NewBuffer[int](
			name+".DirectoryBuffer",
			spec.NumReqPerCycle,
		),
		BankBufs: bankBufs,
		DirPipeline: queueing.NewPipeline[int](
			spec.NumReqPerCycle,
			spec.DirLatency,
		),
		DirPostBuf: queueing.NewBuffer[int](
			name+".DirectoryStage.PostPipelineBuffer",
			spec.NumReqPerCycle,
		),
		BankPipelines: bankPipelines,
		BankPostBufs:  bankPostBufs,
	}

	cache.DirectoryReset(
		&initialState.DirectoryState, numSets, spec.WayAssociativity, blockSize)

	return initialState
}

func (b *Builder) buildPipelineMW(
	comp *modeling.Component[Spec, State, Resources],
) *pipelineMW {
	m := &pipelineMW{
		comp: comp,
	}

	m.storage = comp.Resources().Storage

	return m
}

func (b *Builder) buildStages(m *pipelineMW, spec Spec) {
	m.intakeStage = &intake{cache: m}
	m.directoryStage = &directory{
		cache: m,
	}
	b.buildBankStages(m, spec)
	m.parseBottomStage = &bottomParser{cache: m}
	m.respondStage = &respondStage{cache: m}
}

func (b *Builder) buildBankStages(m *pipelineMW, spec Spec) {
	for i := 0; i < spec.NumBanks; i++ {
		bs := &bankStage{
			cache:          m,
			bankID:         i,
			numReqPerCycle: spec.NumReqPerCycle,
		}
		m.bankStages = append(m.bankStages, bs)
	}
}
