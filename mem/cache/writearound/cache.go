package writearound

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// Comp is a customized L1 cache the for R9nano GPUs.
type Comp struct {
	*sim.TickingComponent

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	numReqPerCycle   int
	log2BlockSize    uint64
	storage          *mem.Storage
	directory        cache.Directory
	mshr             cache.MSHR
	bankLatency      int
	wayAssociativity int
	lowModuleFinder  mem.LowModuleFinder

	dirBuf   sim.Buffer
	bankBufs []sim.Buffer

	coalesceStage    *coalescer
	directoryStage   *directory
	bankStages       []*bankStage
	parseBottomStage *bottomParser
	respondStage     *respondStage
	controlStage     *controlStage

	maxNumConcurrentTrans    int
	transactions             []*transaction
	postCoalesceTransactions []*transaction

	isPaused bool
}

// SetLowModuleFinder sets the finder that tells which remote port can serve
// the data on a certain address.
func (c *Comp) SetLowModuleFinder(lmf mem.LowModuleFinder) {
	c.lowModuleFinder = lmf
}

// Tick update the state of the cache
func (c *Comp) Tick() bool {
	madeProgress := false

	if !c.isPaused {
		madeProgress = c.runPipeline() || madeProgress
	}

	madeProgress = c.controlStage.Tick() || madeProgress

	return madeProgress
}

func (c *Comp) runPipeline() bool {
	madeProgress := false
	madeProgress = c.tickRespondStage() || madeProgress
	madeProgress = c.tickParseBottomStage() || madeProgress
	madeProgress = c.tickBankStage() || madeProgress
	madeProgress = c.tickDirectoryStage() || madeProgress
	madeProgress = c.tickCoalesceState() || madeProgress
	return madeProgress
}

func (c *Comp) tickRespondStage() bool {
	madeProgress := false
	for i := 0; i < c.numReqPerCycle; i++ {
		madeProgress = c.respondStage.Tick() || madeProgress
	}
	return madeProgress
}

func (c *Comp) tickParseBottomStage() bool {
	madeProgress := false

	for i := 0; i < c.numReqPerCycle; i++ {
		madeProgress = c.parseBottomStage.Tick() || madeProgress
	}

	return madeProgress
}

func (c *Comp) tickBankStage() bool {
	madeProgress := false
	for _, bs := range c.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}
	return madeProgress
}

func (c *Comp) tickDirectoryStage() bool {
	return c.directoryStage.Tick()
}

func (c *Comp) tickCoalesceState() bool {
	madeProgress := false
	for i := 0; i < c.numReqPerCycle; i++ {
		madeProgress = c.coalesceStage.Tick() || madeProgress
	}
	return madeProgress
}
