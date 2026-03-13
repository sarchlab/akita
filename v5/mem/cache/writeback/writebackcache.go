package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"
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
	Freq                sim.Freq `json:"freq"`
	NumReqPerCycle      int      `json:"num_req_per_cycle"`
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

	// Address mapper configuration (inlined from interface)
	AddressMapperType string   `json:"address_mapper_type"`
	RemotePortNames   []string `json:"remote_port_names"`
	InterleavingSize  uint64   `json:"interleaving_size"`
}

// State contains mutable runtime data for the writeback cache.
type State struct {
	CacheState     int                  `json:"cache_state"`
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState      `json:"mshr_state"`
	Transactions   []transactionState   `json:"transactions"`
	EvictingList   map[uint64]bool      `json:"evicting_list"`

	// Buffers (transaction indices stored as int)
	DirStageBuf           queueing.Buffer[int]   `json:"dir_stage_buf"`
	DirToBankBufs         []queueing.Buffer[int] `json:"dir_to_bank_bufs"`
	WriteBufferToBankBufs []queueing.Buffer[int] `json:"write_buffer_to_bank_bufs"`
	MSHRStageBuf          queueing.Buffer[int]   `json:"mshr_stage_buf"`
	WriteBufferBuf        queueing.Buffer[int]   `json:"write_buffer_buf"`

	// Directory pipeline + post-buf
	DirPipeline        queueing.Pipeline[int] `json:"dir_pipeline"`
	DirPostPipelineBuf queueing.Buffer[int]   `json:"dir_post_pipeline_buf"`

	// Bank pipeline + post-buf + counters
	BankPipelines                   []queueing.Pipeline[int] `json:"bank_pipelines"`
	BankPostPipelineBufs            []queueing.Buffer[int]   `json:"bank_post_pipeline_bufs"`
	BankInflightTransCounts         []int                     `json:"bank_inflight_trans_counts"`
	BankDownwardInflightTransCounts []int                     `json:"bank_downward_inflight_trans_counts"`

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

// flushReqState is a serializable representation of a cache.FlushReq.
type flushReqState struct {
	MsgMeta                 sim.MsgMeta `json:"msg_meta"`
	InvalidateAllCachelines bool        `json:"invalidate_all_cachelines"`
	DiscardInflight         bool        `json:"discard_inflight"`
	PauseAfterFlushing      bool        `json:"pause_after_flushing"`
}

// pipelineMW holds all non-serializable infrastructure for the writeback
// cache pipeline. It implements the Tick method and delegates NamedHookable
// to comp. All mutable state is in State, accessed via comp.GetNextState().
type pipelineMW struct {
	comp *modeling.Component[Spec, State]

	topPort    sim.Port
	bottomPort sim.Port

	// legacyMapper is kept for backward compatibility with code that sets the
	// mapper's Port field after Build(). When Spec.AddressMapperType is empty,
	// findPort falls back to this runtime mapper.
	legacyMapper mem.AddressToPortMapper

	storage *mem.Storage

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
	return m.comp.GetSpec()
}

// findPort resolves an address to a remote port using data from Spec.
// If the Spec does not contain address mapper configuration (e.g. because
// the port was set after Build via a legacy AddressToPortMapper), it falls
// back to the runtime legacyMapper.
func (m *pipelineMW) findPort(address uint64) sim.RemotePort {
	spec := m.comp.GetSpec()

	switch spec.AddressMapperType {
	case "single":
		if len(spec.RemotePortNames) > 0 {
			name := spec.RemotePortNames[0]
			if name != "" {
				return sim.RemotePort(name)
			}
		}
	case "interleaved":
		if n := uint64(len(spec.RemotePortNames)); n > 0 {
			idx := address / spec.InterleavingSize % n
			name := spec.RemotePortNames[idx]
			if name != "" {
				return sim.RemotePort(name)
			}
		}
	}

	// Fall back to legacy runtime mapper if available.
	if m.legacyMapper != nil {
		return m.legacyMapper.Find(address)
	}

	panic("findPort: no valid address mapping for address; " +
		"Spec.AddressMapperType=" + spec.AddressMapperType)
}

// Tick updates the internal states of the Cache pipeline.
func (m *pipelineMW) Tick() bool {
	next := m.comp.GetNextState()
	madeProgress := false

	if cacheState(next.CacheState) != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

// syncForTest synchronizes state so that GetNextState reflects prior mutations.
// In tests, state is set up via GetNextState() without going through
// Component.Tick(), so we commit and re-derive.
func (m *pipelineMW) syncForTest() {
	next := m.comp.GetNextState()
	m.comp.SetState(*next)
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false

	spec := m.comp.GetSpec()

	madeProgress = m.runStage(m.mshrStage, spec.NumReqPerCycle) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.dirStage, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.topParser, spec.NumReqPerCycle) || madeProgress

	return madeProgress
}

func (m *pipelineMW) runStage(stage sim.Ticker, n int) bool {
	madeProgress := false
	for range n {
		madeProgress = stage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) discardInflightTransactions() {
	next := m.comp.GetNextState()

	for i := range next.DirectoryState.Sets {
		for j := range next.DirectoryState.Sets[i].Blocks {
			next.DirectoryState.Sets[i].Blocks[j].ReadCount = 0
			next.DirectoryState.Sets[i].Blocks[j].IsLocked = false
		}
	}

	// Clear all buffers and pipelines
	next.DirStageBuf.Clear()
	next.DirPipeline.Stages = nil
	next.DirPostPipelineBuf.Clear()
	for i := range next.DirToBankBufs {
		next.DirToBankBufs[i].Clear()
	}
	for i := range next.WriteBufferToBankBufs {
		next.WriteBufferToBankBufs[i].Clear()
	}
	for i := range next.BankPipelines {
		next.BankPipelines[i].Stages = nil
	}
	for i := range next.BankPostPipelineBufs {
		next.BankPostPipelineBufs[i].Clear()
	}
	next.MSHRStageBuf.Clear()
	next.WriteBufferBuf.Clear()

	for i := range next.BankInflightTransCounts {
		next.BankInflightTransCounts[i] = 0
		next.BankDownwardInflightTransCounts[i] = 0
	}

	// Clear MSHR stage state
	next.HasProcessingMSHREntry = false
	next.ProcessingMSHREntryIdx = 0

	// Clear write buffer stage state
	next.PendingEvictionIndices = nil
	next.InflightFetchIndices = nil
	next.InflightEvictionIndices = nil

	clearPort(m.topPort)

	next.Transactions = nil
}

// controlMW runs the flusher (flush/invalidate from controlPort,
// controls cache state).
type controlMW struct {
	comp    *modeling.Component[Spec, State]
	flusher *flusher
}

// Tick runs the flusher.
func (m *controlMW) Tick() bool {
	return m.flusher.Tick()
}
