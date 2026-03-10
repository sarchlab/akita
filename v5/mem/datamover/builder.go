package datamover

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder for StreamingDataMover
type Builder struct {
	engine                 sim.Engine
	bufferSize             uint64
	insidePortMapper       mem.AddressToPortMapper
	outsidePortMapper      mem.AddressToPortMapper
	insideByteGranularity  uint64
	outsideByteGranularity uint64
	ctrlPort               sim.Port
	insidePort             sim.Port
	outsidePort            sim.Port
}

// MakeBuilder creates a new Builder
func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets StreamingDataMover's engine
func (sdmBuilder Builder) WithEngine(
	inputEngine sim.Engine,
) Builder {
	sdmBuilder.engine = inputEngine
	return sdmBuilder
}

// WithBufferSize sets the buffer size of StreamingDataMover
func (sdmBuilder Builder) WithBufferSize(
	inputBufferSize uint64,
) Builder {
	sdmBuilder.bufferSize = inputBufferSize
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
	sdmBuilder.insideByteGranularity = inputInsideByteGranularity
	return sdmBuilder
}

// WithOutsideByteGranularity sets the outside byte granularity of
// StreamingDataMover
func (sdmBuilder Builder) WithOutsideByteGranularity(
	inputOutsideByteGranularity uint64,
) Builder {
	sdmBuilder.outsideByteGranularity = inputOutsideByteGranularity
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
	sdm := &Comp{}
	sdm.bufferSize = sdmBuilder.bufferSize
	sdm.insidePortMapper = sdmBuilder.insidePortMapper
	sdm.outsidePortMapper = sdmBuilder.outsidePortMapper
	sdm.insideByteGranularity = sdmBuilder.insideByteGranularity
	sdm.outsideByteGranularity = sdmBuilder.outsideByteGranularity

	sdm.TickingComponent = sim.NewTickingComponent(
		name, sdmBuilder.engine, 1*sim.GHz, sdm)

	sdm.ctrlPort = sdmBuilder.ctrlPort
	sdm.ctrlPort.SetComponent(sdm)
	sdm.insidePort = sdmBuilder.insidePort
	sdm.insidePort.SetComponent(sdm)
	sdm.outsidePort = sdmBuilder.outsidePort
	sdm.outsidePort.SetComponent(sdm)

	return sdm
}
