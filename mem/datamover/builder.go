package datamover

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder for StreamingDataMover
type Builder struct {
	name            string
	engine          sim.Engine
	bufferSize      int
	localDataSource mem.AddressToPortMapper
}

// MakeBuilder creates a new Builder
func MakeBuilder() Builder {
	return Builder{}
}

// WithName sets the name of StreamingDataMover's ticking component
func (sdmBuilder Builder) WithName(
	inputName string,
) Builder {
	sdmBuilder.name = inputName
	return sdmBuilder
}

// WithEngine sets StreamingDataMover's engine
func (sdmBuilder Builder) WithEngine(
	inputEngine sim.Engine,
) Builder {
	sdmBuilder.engine = inputEngine
	return sdmBuilder
}

// WithLocalDataSource sets the local data source of StreamingDataMover
func (sdmBuilder Builder) WithLocalDataSource(
	inputLocaDataSource mem.AddressToPortMapper,
) Builder {
	sdmBuilder.localDataSource = inputLocaDataSource
	return sdmBuilder
}

// WithBufferSize sets the buffer size of StreamingDataMover
func (sdmBuilder Builder) WithBufferSize(
	inputBufferSize int,
) Builder {
	sdmBuilder.bufferSize = inputBufferSize
	return sdmBuilder
}

// Build a new StreamingDataMover
func (sdmBuilder Builder) Build() *StreamingDataMover {
	sdm := &StreamingDataMover{}
	sdm.buffer = make([]byte, sdmBuilder.bufferSize)
	sdm.localDataSource = sdmBuilder.localDataSource
	sdm.TickingComponent = sim.NewTickingComponent(
		sdmBuilder.name, sdmBuilder.engine, 1*sim.GHz, sdm)

	sdm.ctrlPort = sim.NewLimitNumMsgPort(sdm, 40960000, sdmBuilder.name+".CtrlPort")
	sdm.insidePort = sim.NewLimitNumMsgPort(sdm, 64, sdmBuilder.name+".SrcPort")
	sdm.outsidePort = sim.NewLimitNumMsgPort(sdm, 64, sdmBuilder.name+".DstPort")

	return sdm
}
