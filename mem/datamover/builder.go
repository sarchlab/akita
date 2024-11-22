package datamoving

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder for StreamingDataMover
type Builder struct {
	name            string
	engine          sim.Engine
	bufferSize      int
	localDataSource mem.LowModuleFinder
}

// WithName sets the name of StreamingDataMover's ticking component
func (sdmBuilder *Builder) WithName(
	inputName string,
) {
	sdmBuilder.name = inputName
}

// WithEngine sets StreamingDataMover's engine
func (sdmBuilder *Builder) WithEngine(
	inputEngine sim.Engine,
) {
	sdmBuilder.engine = inputEngine
}

// WithLocalDataSource sets the local data source of StreamingDataMover
func (sdmBuilder *Builder) WithLocalDataSource(
	inputLocaDataSource mem.LowModuleFinder,
) {
	sdmBuilder.localDataSource = inputLocaDataSource
}

// WithBufferSize sets the buffer size of StreamingDataMover
func (sdmBuilder *Builder) WithBufferSize(
	inputBufferSize int,
) {
	sdmBuilder.bufferSize = inputBufferSize
}

// Build a new StreamingDataMover
func (sdmBuilder *Builder) Build() *StreamingDataMover {
	sdm := &StreamingDataMover{}
	sdm.buffer = make([]byte, sdmBuilder.bufferSize)
	sdm.localDataSource = sdmBuilder.localDataSource
	sdm.TickingComponent = sim.NewTickingComponent(
		sdmBuilder.name, sdmBuilder.engine, 1*sim.GHz, sdm)

	sdm.CtrlPort = sim.NewLimitNumMsgPort(sdm, 40960000, sdmBuilder.name+".CtrlPort")
	sdm.SrcPort = sim.NewLimitNumMsgPort(sdm, 64, sdmBuilder.name+".SrcPort")
	sdm.DstPort = sim.NewLimitNumMsgPort(sdm, 64, sdmBuilder.name+".DstPort")

	return sdm
}
