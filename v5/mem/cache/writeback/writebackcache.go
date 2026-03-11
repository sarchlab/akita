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
type Spec struct {
	NumReqPerCycle      int    `json:"num_req_per_cycle"`
	Log2BlockSize       uint64 `json:"log2_block_size"`
	BankLatency         int    `json:"bank_latency"`
	WayAssociativity    int    `json:"way_associativity"`
	NumBanks            int    `json:"num_banks"`
	NumSets             int    `json:"num_sets"`
	NumMSHREntry        int    `json:"num_mshr_entry"`
	TotalByteSize       uint64 `json:"total_byte_size"`
	DirLatency          int    `json:"dir_latency"`
	WriteBufferCapacity int    `json:"write_buffer_capacity"`
	MaxInflightFetch    int    `json:"max_inflight_fetch"`
	MaxInflightEviction int    `json:"max_inflight_eviction"`
}

// State contains mutable runtime data for the writeback cache.
type State struct {
	CacheState     int                  `json:"cache_state"`
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState      `json:"mshr_state"`
	Transactions   []transactionSnapshot `json:"transactions"`
	EvictingList   map[uint64]bool      `json:"evicting_list"`

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

// middleware holds all non-serializable infrastructure for the writeback
// cache. It implements the Tick method and delegates NamedHookable to comp.
type middleware struct {
	comp *modeling.Component[Spec, State]

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	storage             *mem.Storage
	directoryState      cache.DirectoryState // runtime copy
	mshrState           cache.MSHRState      // runtime copy
	addressToPortMapper mem.AddressToPortMapper

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

	log2BlockSize    uint64
	numReqPerCycle   int
	wayAssociativity int
	numMSHREntry     int
	numSets          int
	blockSize        int

	state                cacheState
	inFlightTransactions []*transactionState
	evictingList         map[uint64]bool
}

// --- NamedHookable delegation ---

func (m *middleware) Name() string {
	return m.comp.Name()
}

func (m *middleware) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *middleware) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *middleware) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *middleware) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

// GetSpec returns the immutable specification.
func (m *middleware) GetSpec() Spec {
	return m.comp.GetSpec()
}

// SetAddressToPortMapper sets the AddressToPortMapper used by the cache.
func (m *middleware) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	m.addressToPortMapper = lmf
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

func (m *middleware) discardInflightTransactions() {
	for i := range m.directoryState.Sets {
		for j := range m.directoryState.Sets[i].Blocks {
			m.directoryState.Sets[i].Blocks[j].ReadCount = 0
			m.directoryState.Sets[i].Blocks[j].IsLocked = false
		}
	}

	m.dirStage.Reset()

	for _, bs := range m.bankStages {
		bs.Reset()
	}

	m.mshrStage.Reset()
	m.writeBuffer.Reset()

	clearPort(m.topPort)

	m.inFlightTransactions = nil
}
