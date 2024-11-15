package datamoving

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder for StreamingDataMover
type Builder struct {
	name            string
	engine          sim.Engine
	Log2AccessSize  uint64
	localDataSource mem.LowModuleFinder
}

// Sets the name of StreamingDataMover's ticking component
func (sdmBuilder *Builder) WithName(
	inputName string,
) {
	sdmBuilder.name = inputName
}

// Sets StreamingDataMover's engine
func (sdmBuilder *Builder) WithEngine(
	inputEngine sim.Engine,
) {
	sdmBuilder.engine = inputEngine
}

// Sets
func (sdmBuilder *Builder) WithByteSize(
	inputByteSize uint64,
) {
	sdmBuilder.Log2AccessSize = inputByteSize
}

// Sets the local data source of StreamingDataMover
func (sdmBuilder *Builder) WithLocalDataSource(
	inputLocaDataSource mem.LowModuleFinder,
) {
	sdmBuilder.localDataSource = inputLocaDataSource
}

// Creates a new StreamingDataMover
func (sdmBuilder *Builder) Build() *StreamingDataMover {
	sdm := &StreamingDataMover{}
	sdm.buffer = []byte{}
	sdm.localDataSource = sdmBuilder.localDataSource
	sdm.Log2AccessSize = sdmBuilder.Log2AccessSize
	sdm.TickingComponent = sim.NewTickingComponent(
		sdmBuilder.name, sdmBuilder.engine, 1*sim.GHz, sdm)

	sdm.CtrlPort = sim.NewLimitNumMsgPort(sdm, 40960000, sdmBuilder.name+".CtrlPort")
	sdm.SrcPort = sim.NewLimitNumMsgPort(sdm, 64, sdmBuilder.name+".SrcPort")
	sdm.DstPort = sim.NewLimitNumMsgPort(sdm, 64, sdmBuilder.name+".DstPort")

	return sdm
}
