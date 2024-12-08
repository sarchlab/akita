package writeback

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"

	"github.com/sarchlab/akita/v4/sim"
)

type cacheState int

const (
	cacheStateInvalid cacheState = iota
	cacheStateRunning
	cacheStatePreFlushing
	cacheStateFlushing
	cacheStatePaused
)

// Comp in the writeback package is a cache that performs the write-back policy.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	dirStageBuffer           sim.Buffer
	dirToBankBuffers         []sim.Buffer
	writeBufferToBankBuffers []sim.Buffer
	mshrStageBuffer          sim.Buffer
	writeBufferBuffer        sim.Buffer

	topSender         sim.BufferedSender
	bottomSender      sim.BufferedSender
	controlPortSender sim.BufferedSender

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
	flusher     *flusher

	storage         *mem.Storage
	lowModuleFinder mem.AddressToPortMapper
	directory       cache.Directory
	mshr            cache.MSHR
	log2BlockSize   uint64
	numReqPerCycle  int

	state                cacheState
	inFlightTransactions []*transaction
	evictingList         map[uint64]bool
}

// SetLowModuleFinder sets the LowModuleFinder used by the cache.
func (c *Comp) SetLowModuleFinder(lmf mem.AddressToPortMapper) {
	c.lowModuleFinder = lmf
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates the internal states of the Cache.
func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.controlPortSender.Tick() || madeProgress

	if m.state != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	madeProgress = m.flusher.Tick() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

	madeProgress = m.runStage(m.topSender) || madeProgress
	madeProgress = m.runStage(m.bottomSender) || madeProgress
	madeProgress = m.runStage(m.mshrStage) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer) || madeProgress
	madeProgress = m.runStage(m.dirStage) || madeProgress
	madeProgress = m.runStage(m.topParser) || madeProgress

	return madeProgress
}

func (m *middleware) runStage(stage sim.Ticker) bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = stage.Tick() || madeProgress
	}
	return madeProgress
}

func (c *Comp) discardInflightTransactions() {
	sets := c.directory.GetSets()
	for _, set := range sets {
		for _, block := range set.Blocks {
			block.ReadCount = 0
			block.IsLocked = false
		}
	}

	c.dirStage.Reset()
	for _, bs := range c.bankStages {
		bs.Reset()
	}
	c.mshrStage.Reset()
	c.writeBuffer.Reset()

	clearPort(c.topPort)

	c.topSender.Clear()

	// for _, t := range c.inFlightTransactions {
	// 	fmt.Printf("%.10f, %s, transaction %s discarded due to flushing\n",
	// 		now, c.Name(), t.id)
	// }

	c.inFlightTransactions = nil
}
