package datamover

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Builder for StreamingDataMover
type Builder struct {
	simulation             simulation.Simulation
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

// WithSimulation sets StreamingDataMover's simulation
func (sdmBuilder Builder) WithSimulation(
	inputSimulation simulation.Simulation,
) Builder {
	sdmBuilder.simulation = inputSimulation
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
	sdm.state = &state{
		name: name,
	}

	sdmBuilder.simulation.RegisterStateHolder(sdm)

	sdm.TickingComponent = modeling.NewTickingComponent(
		name, sdmBuilder.simulation.GetEngine(), 1*timing.GHz, sdm)

	sdm.ctrlPort = modeling.PortBuilder{}.
		WithComponent(sdm).
		WithSimulation(sdmBuilder.simulation).
		WithIncomingBufCap(4).
		WithOutgoingBufCap(4).
		Build(name + ".CtrlPort")
	sdm.AddPort("CtrlPort", sdm.ctrlPort)

	sdm.insidePort = modeling.PortBuilder{}.
		WithComponent(sdm).
		WithSimulation(sdmBuilder.simulation).
		WithIncomingBufCap(64).
		WithOutgoingBufCap(64).
		Build(name + ".SrcPort")
	sdm.AddPort("SrcPort", sdm.insidePort)

	sdm.outsidePort = modeling.PortBuilder{}.
		WithComponent(sdm).
		WithSimulation(sdmBuilder.simulation).
		WithIncomingBufCap(64).
		WithOutgoingBufCap(64).
		Build(name + ".DstPort")
	sdm.AddPort("DstPort", sdm.outsidePort)

	return sdm
}
