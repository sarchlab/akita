package writeevict

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
)

// Comp is a customized L1 cache the for R9nano GPUs.
type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder

	topPort     modeling.Port
	bottomPort  modeling.Port
	controlPort modeling.Port

	numReqPerCycle      int
	log2BlockSize       uint64
	storage             *mem.Storage
	directory           cache.Directory
	mshr                cache.MSHR
	bankLatency         int
	wayAssociativity    int
	addressToPortMapper mem.AddressToPortMapper

	dirBuf   queueing.Buffer
	bankBufs []queueing.Buffer

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

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

// SetAddressToPortMapper sets the finder that tells which remote port can serve
// the data on a certain address.
func (c *Comp) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	c.addressToPortMapper = lmf
}

type middleware struct {
	*Comp
}

// Tick update the state of the cache
func (m *middleware) Tick() bool {
	madeProgress := false

	if !m.isPaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	madeProgress = m.controlStage.Tick() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false
	madeProgress = m.tickRespondStage() || madeProgress
	madeProgress = m.tickParseBottomStage() || madeProgress
	madeProgress = m.tickBankStage() || madeProgress
	madeProgress = m.tickDirectoryStage() || madeProgress
	madeProgress = m.tickCoalesceState() || madeProgress

	return madeProgress
}

func (m *middleware) tickRespondStage() bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respondStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickParseBottomStage() bool {
	madeProgress := false

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.parseBottomStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickBankStage() bool {
	madeProgress := false
	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickDirectoryStage() bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.directoryStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickCoalesceState() bool {
	return m.coalesceStage.Tick()
}
