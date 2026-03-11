package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the writearound cache.
type Spec struct {
	NumReqPerCycle        int    `json:"num_req_per_cycle"`
	Log2BlockSize         uint64 `json:"log2_block_size"`
	BankLatency           int    `json:"bank_latency"`
	WayAssociativity      int    `json:"way_associativity"`
	MaxNumConcurrentTrans int    `json:"max_num_concurrent_trans"`
	NumBanks              int    `json:"num_banks"`
	NumMSHREntry          int    `json:"num_mshr_entry"`
	NumSets               int    `json:"num_sets"`
	TotalByteSize         uint64 `json:"total_byte_size"`
	DirLatency            int    `json:"dir_latency"`
}

// State contains mutable runtime data for the writearound cache.
type State struct {
	DirectoryState             cache.DirectoryState       `json:"directory_state"`
	MSHRState                  cache.MSHRState            `json:"mshr_state"`
	Transactions               []transactionSnapshot      `json:"transactions"`
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

	storage             *mem.Storage
	directory           cache.Directory
	mshr                cache.MSHR
	addressToPortMapper mem.AddressToPortMapper

	dirBuf   queueing.Buffer
	bankBufs []queueing.Buffer

	coalesceStage    *coalescer
	directoryStage   *directory
	bankStages       []*bankStage
	parseBottomStage *bottomParser
	respondStage     *respondStage
	controlStage     *controlStage

	transactions             []*transactionState
	postCoalesceTransactions []*transactionState

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
	spec := m.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickParseBottomStage() bool {
	madeProgress := false

	spec := m.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
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
	return m.directoryStage.Tick()
}

func (m *middleware) tickCoalesceState() bool {
	madeProgress := false
	spec := m.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.coalesceStage.Tick() || madeProgress
	}

	return madeProgress
}
