package datamover

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder for StreamingDataMover
type Builder struct {
	engine      sim.Engine
	freq        sim.Freq
	ctrlPort    sim.Port
	insidePort  sim.Port
	outsidePort sim.Port
	spec        *Spec
}

// MakeBuilder creates a new Builder
func MakeBuilder() Builder {
	return Builder{
		freq: 1 * sim.GHz,
		spec: &Spec{},
	}
}

// WithEngine sets StreamingDataMover's engine
func (sdmBuilder Builder) WithEngine(
	inputEngine sim.Engine,
) Builder {
	sdmBuilder.engine = inputEngine
	return sdmBuilder
}

// WithFreq sets the frequency of StreamingDataMover
func (sdmBuilder Builder) WithFreq(freq sim.Freq) Builder {
	sdmBuilder.freq = freq
	return sdmBuilder
}

// WithBufferSize sets the buffer size of StreamingDataMover
func (sdmBuilder Builder) WithBufferSize(
	inputBufferSize uint64,
) Builder {
	sdmBuilder.spec.BufferSize = inputBufferSize
	return sdmBuilder
}

// WithInsidePortMapper sets the inside port mapper of StreamingDataMover.
// It inlines the mapper configuration into the Spec.
func (sdmBuilder Builder) WithInsidePortMapper(
	inputInsidePortMapper mem.AddressToPortMapper,
) Builder {
	inlineMapper(inputInsidePortMapper,
		&sdmBuilder.spec.InsideMapperKind,
		&sdmBuilder.spec.InsideMapperPorts,
		&sdmBuilder.spec.InsideMapperInterleavingSize)
	return sdmBuilder
}

// WithOutsidePortMapper sets the outside port mapper of StreamingDataMover.
// It inlines the mapper configuration into the Spec.
func (sdmBuilder Builder) WithOutsidePortMapper(
	inputOutsidePortMapper mem.AddressToPortMapper,
) Builder {
	inlineMapper(inputOutsidePortMapper,
		&sdmBuilder.spec.OutsideMapperKind,
		&sdmBuilder.spec.OutsideMapperPorts,
		&sdmBuilder.spec.OutsideMapperInterleavingSize)
	return sdmBuilder
}

// WithInsideByteGranularity sets the inside byte granularity of
// StreamingDataMover
func (sdmBuilder Builder) WithInsideByteGranularity(
	inputInsideByteGranularity uint64,
) Builder {
	sdmBuilder.spec.InsideByteGranularity = inputInsideByteGranularity
	return sdmBuilder
}

// WithOutsideByteGranularity sets the outside byte granularity of
// StreamingDataMover
func (sdmBuilder Builder) WithOutsideByteGranularity(
	inputOutsideByteGranularity uint64,
) Builder {
	sdmBuilder.spec.OutsideByteGranularity = inputOutsideByteGranularity
	return sdmBuilder
}

// WithCtrlPort sets the control port of StreamingDataMover
func (sdmBuilder Builder) WithCtrlPort(port sim.Port) Builder {
	sdmBuilder.ctrlPort = port
	return sdmBuilder
}

// WithInsidePort sets the inside port of StreamingDataMover
func (sdmBuilder Builder) WithInsidePort(port sim.Port) Builder {
	sdmBuilder.insidePort = port
	return sdmBuilder
}

// WithOutsidePort sets the outside port of StreamingDataMover
func (sdmBuilder Builder) WithOutsidePort(port sim.Port) Builder {
	sdmBuilder.outsidePort = port
	return sdmBuilder
}

// Build a new StreamingDataMover
func (sdmBuilder Builder) Build(name string) *modeling.Component[Spec, State] {
	spec := *sdmBuilder.spec
	initialState := State{}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(sdmBuilder.engine).
		WithFreq(sdmBuilder.freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	ctrlMW := &ctrlParseMW{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	dataMW := &dataTransferMW{comp: modelComp}
	modelComp.AddMiddleware(dataMW)

	sdmBuilder.ctrlPort.SetComponent(modelComp)
	modelComp.AddPort("Control", sdmBuilder.ctrlPort)

	sdmBuilder.insidePort.SetComponent(modelComp)
	modelComp.AddPort("Inside", sdmBuilder.insidePort)

	sdmBuilder.outsidePort.SetComponent(modelComp)
	modelComp.AddPort("Outside", sdmBuilder.outsidePort)

	return modelComp
}

// inlineMapper converts an AddressToPortMapper into serializable Spec fields.
func inlineMapper(
	mapper mem.AddressToPortMapper,
	kind *string,
	ports *[]sim.RemotePort,
	interleavingSize *uint64,
) {
	switch m := mapper.(type) {
	case *mem.SinglePortMapper:
		*kind = "single"
		*ports = []sim.RemotePort{m.Port}
		*interleavingSize = 0
	case *mem.InterleavedAddressPortMapper:
		*kind = "interleaved"
		*ports = make([]sim.RemotePort, len(m.LowModules))
		copy(*ports, m.LowModules)
		*interleavingSize = m.InterleavingSize
	default:
		panic("unsupported mapper type for inline conversion")
	}
}
