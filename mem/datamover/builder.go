package datamover

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Builder for StreamingDataMover
type Builder struct {
	engine                 timing.Engine
	bufferSize             uint64
	insidePortMapper       mem.AddressToPortMapper
	outsidePortMapper      mem.AddressToPortMapper
	insideByteGranularity  uint64
	outsideByteGranularity uint64
}

// MakeBuilder creates a new Builder
func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets StreamingDataMover's engine
func (sdmBuilder Builder) WithEngine(
	inputEngine timing.Engine,
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

// Build a new StreamingDataMover
func (sdmBuilder Builder) Build(name string) *Comp {
	sdm := &Comp{}
	sdm.bufferSize = sdmBuilder.bufferSize
	sdm.insidePortMapper = sdmBuilder.insidePortMapper
	sdm.outsidePortMapper = sdmBuilder.outsidePortMapper
	sdm.insideByteGranularity = sdmBuilder.insideByteGranularity
	sdm.outsideByteGranularity = sdmBuilder.outsideByteGranularity

	sdm.TickingComponent = modeling.NewTickingComponent(
		name, sdmBuilder.engine, 1*timing.GHz, sdm)

	sdm.ctrlPort = modeling.NewPort(sdm, 40960000, 40960000, name+".CtrlPort")
	sdm.insidePort = modeling.NewPort(sdm, 64, 64, name+".SrcPort")
	sdm.outsidePort = modeling.NewPort(sdm, 64, 64, name+".DstPort")

	return sdm
}
