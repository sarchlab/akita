package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"

	"github.com/sarchlab/akita/v5/sim"
)

type cacheState int

const (
	cacheStateInvalid cacheState = iota
	cacheStateRunning
	cacheStatePreFlushing
	cacheStateFlushing
	cacheStatePaused
)

// Spec contains immutable configuration for the writeback cache.
type Spec struct{}

// State contains mutable runtime data for the writeback cache.
type State struct {
	CacheState     int                `json:"cache_state"`
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState    `json:"mshr_state"`
	Transactions   []transactionState `json:"transactions"`
	EvictingList   map[uint64]bool    `json:"evicting_list"`

	// 5 buffer snapshots (transaction indices)
	DirStageBufIndices          []int          `json:"dir_stage_buf_indices"`
	DirToBankBufIndices         []bankBufState `json:"dir_to_bank_buf_indices"`
	WriteBufferToBankBufIndices []bankBufState `json:"write_buffer_to_bank_buf_indices"`
	MSHRStageBufEntries         []int          `json:"mshr_stage_buf_entries"`
	WriteBufferBufIndices       []int          `json:"write_buffer_buf_indices"`

	// Directory pipeline + post-buf
	DirPipelineStages         []dirPipelineStageState `json:"dir_pipeline_stages"`
	DirPostPipelineBufIndices []int                   `json:"dir_post_pipeline_buf_indices"`

	// Bank pipeline + post-buf + counters
	BankPipelineStages              []bankPipelineState `json:"bank_pipeline_stages"`
	BankPostPipelineBufIndices      []bankPostBufState  `json:"bank_post_pipeline_buf_indices"`
	BankInflightTransCounts         []int               `json:"bank_inflight_trans_counts"`
	BankDownwardInflightTransCounts []int               `json:"bank_downward_inflight_trans_counts"`

	// Write buffer stage
	PendingEvictionIndices  []int `json:"pending_eviction_indices"`
	InflightFetchIndices    []int `json:"inflight_fetch_indices"`
	InflightEvictionIndices []int `json:"inflight_eviction_indices"`

	// MSHR stage
	HasProcessingMSHREntry bool `json:"has_processing_mshr_entry"`
	ProcessingMSHREntryIdx int  `json:"processing_mshr_entry_idx"`

	// Flusher
	FlusherBlockToEvictRefs []blockRef    `json:"flusher_block_to_evict_refs"`
	HasProcessingFlush      bool          `json:"has_processing_flush"`
	ProcessingFlush         flushReqState `json:"processing_flush"`
}

// Comp in the writeback package is a cache that performs the write-back policy.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	dirStageBuffer           queueing.Buffer
	dirToBankBuffers         []queueing.Buffer
	writeBufferToBankBuffers []queueing.Buffer
	mshrStageBuffer          queueing.Buffer
	writeBufferBuffer        queueing.Buffer

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
	flusher     *flusher

	storage             *mem.Storage
	addressToPortMapper mem.AddressToPortMapper
	directory           cache.Directory
	mshr                cache.MSHR
	log2BlockSize       uint64
	numReqPerCycle      int

	state                cacheState
	inFlightTransactions []*transaction
	evictingList         map[uint64]bool
}

// SetAddressToPortMapper sets the AddressToPortMapper used by the cache.
func (c *Comp) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	c.addressToPortMapper = lmf
}

type middleware struct {
	*Comp
}

// Tick updates the internal states of the Cache.
func (m *middleware) Tick() bool {
	madeProgress := false

	if m.state != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	madeProgress = m.flusher.Tick() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

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

	// for _, t := range c.inFlightTransactions {
	// 	fmt.Printf("%.10f, %s, transaction %s discarded due to flushing\n",
	// 		now, c.Name(), t.id)
	// }

	c.inFlightTransactions = nil
}
