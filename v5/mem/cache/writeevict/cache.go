package writeevict

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"

	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the write-evict cache.
type Spec struct{}

// State contains mutable runtime data for the write-evict cache.
type State struct {
	DirectoryState             cache.DirectoryState       `json:"directory_state"`
	MSHRState                  cache.MSHRState            `json:"mshr_state"`
	Transactions               []transactionState         `json:"transactions"`
	NumTransactions            int                        `json:"num_transactions"`
	DirBufIndices              []int                      `json:"dir_buf_indices"`
	BankBufIndices             []bankBufState             `json:"bank_buf_indices"`
	DirPipelineStages          []dirPipelineStageState    `json:"dir_pipeline_stages"`
	DirPostPipelineBufIndices  []int                      `json:"dir_post_pipeline_buf_indices"`
	BankPipelineStages         []bankPipelineState        `json:"bank_pipeline_stages"`
	BankPostPipelineBufIndices []bankPostBufState         `json:"bank_post_pipeline_buf_indices"`
	IsPaused                   bool                       `json:"is_paused"`
}

// Comp is a customized L1 cache the for R9nano GPUs.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

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
