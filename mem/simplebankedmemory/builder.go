package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides default configuration for the simple banked memory.
var defaultSpec = Spec{
	Freq:                           1 * timing.GHz,
	NumBanks:                       4,
	BankPipelineWidth:              1,
	BankPipelineDepth:              1,
	StageLatency:                   10,
	PostPipelineBufSize:            1,
	TopPortBufferSize:              16,
	CtrlPortBufferSize:             4,
	Capacity:                       4 * mem.GB,
	BankSelectorKind:               "interleaved",
	BankSelectorLog2InterleaveSize: 6,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder constructs SimpleBankedMemory components. Configuration is supplied
// as a whole through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component creates its own ports.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
}

// MakeBuilder creates a builder seeded with the default spec.
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

// WithResources injects the component's shared resources (e.g. a storage
// shared with other components). If not set, the component builds its own,
// sized by Spec.Capacity.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build creates a SimpleBankedMemory component. It creates the component's Top
// port.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("simplebankedmemory: WithRegistrar is required")
	}

	spec := b.spec
	spec.StorageRef = name + ".Storage"

	storage := b.resolveStorage(name, spec)
	initialState := buildInitialState(spec)

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(Resources{Storage: storage}).
		Build(name)
	modelComp.State = initialState

	topPort := messaging.NewPort(
		modelComp, spec.TopPortBufferSize, spec.TopPortBufferSize, name+".Top")
	modelComp.AddPort("Top", topPort)

	ctrlBuf := spec.CtrlPortBufferSize
	if ctrlBuf == 0 {
		ctrlBuf = 4
	}
	ctrlPort := messaging.NewPort(
		modelComp, ctrlBuf, ctrlBuf, name+".Control")
	modelComp.AddPort("Control", ctrlPort)

	cMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cMW)
	tfMW := &tickFinalizeMW{comp: modelComp}
	modelComp.AddMiddleware(tfMW)
	dMW := &dispatchMW{comp: modelComp}
	modelComp.AddMiddleware(dMW)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}

// resolveStorage returns the injected storage, or builds a default one sized by
// Spec.Capacity that self-registers with the registrar.
func (b Builder) resolveStorage(name string, spec Spec) *mem.Storage {
	if b.resources.Storage != nil {
		return b.resources.Storage
	}

	return mem.MakeStorageBuilder().
		WithCapacity(spec.Capacity).
		WithSimulation(b.registrar).
		Build(name + ".Storage")
}

func buildInitialState(spec Spec) State {
	return State{
		Banks: buildInitialBanks(spec),
	}
}

func buildInitialBanks(spec Spec) []bankState {
	banks := make([]bankState, spec.NumBanks)
	for i := range banks {
		banks[i] = bankState{
			Pipeline: queueing.NewPipeline[bankPipelineItemState](
				spec.BankPipelineWidth,
				spec.BankPipelineDepth*spec.StageLatency,
			),
			PostPipelineBuf: queueing.NewBuffer[bankPipelineItemState](
				spec.StorageRef+".PostPipelineBuf",
				spec.PostPipelineBufSize,
			),
		}
	}
	return banks
}
