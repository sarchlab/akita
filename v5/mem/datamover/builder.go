package datamover

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder for StreamingDataMover
type Builder struct {
	engine            sim.Engine
	freq              sim.Freq
	insidePortMapper  mem.AddressToPortMapper
	outsidePortMapper mem.AddressToPortMapper
	ctrlPort          sim.Port
	insidePort        sim.Port
	outsidePort       sim.Port
	spec              *Spec
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

// WithInsidePortMapper sets the inside port mapper of StreamingDataMover
func (sdmBuilder Builder) WithInsidePortMapper(
	inputInsidePortMapper mem.AddressToPortMapper,
) Builder {
	sdmBuilder.insidePortMapper = inputInsidePortMapper
	return sdmBuilder
}

// WithOutsidePortMapper sets the outside port mapper of StreamingDataMover
func (sdmBuilder Builder) WithOutsidePortMapper(
	inputOutsidePortMapper mem.AddressToPortMapper,
) Builder {
	sdmBuilder.outsidePortMapper = inputOutsidePortMapper
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
func (sdmBuilder Builder) Build(name string) *Comp {
	spec := *sdmBuilder.spec
	initialState := State{}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(sdmBuilder.engine).
		WithFreq(sdmBuilder.freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	sdm := &Comp{
		Component:         modelComp,
		insidePortMapper:  sdmBuilder.insidePortMapper,
		outsidePortMapper: sdmBuilder.outsidePortMapper,
	}

	middleware := &dataMoverMiddleware{Comp: sdm}
	sdm.AddMiddleware(middleware)

	sdm.ctrlPort = sdmBuilder.ctrlPort
	sdm.ctrlPort.SetComponent(sdm)
	sdm.AddPort("Control", sdm.ctrlPort)

	sdm.insidePort = sdmBuilder.insidePort
	sdm.insidePort.SetComponent(sdm)
	sdm.AddPort("Inside", sdm.insidePort)

	sdm.outsidePort = sdmBuilder.outsidePort
	sdm.outsidePort.SetComponent(sdm)
	sdm.AddPort("Outside", sdm.outsidePort)

	return sdm
}
