package writeback

import (
	"maps"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
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

// pipelineMW holds all non-serializable infrastructure for the writeback
// cache pipeline. It implements the Tick method and delegates NamedHookable
// to comp.
type pipelineMW struct {
	comp *modeling.Component[Spec, State]

	topPort    sim.Port
	bottomPort sim.Port

	storage *mem.Storage

	// curState holds the A-buffer snapshot for the current tick.
	// Stored here so adapter read pointers remain valid for the tick duration.
	// In production, set by updateAdapterPointers() from comp.GetState().
	curState State

	// Thin buffer adapters (created once, pointers updated per-tick)
	dirStageBuffer           *stateTransBuffer
	dirToBankBuffers         []*stateTransBuffer
	writeBufferToBankBuffers []*stateTransBuffer
	mshrStageBuffer          *stateTransBuffer
	writeBufferBuffer        *stateTransBuffer

	// Dir pipeline/post-buf adapters
	dirPostBufAdapter *stateDirPostBufAdapter

	// Bank pipeline/post-buf adapters
	bankPostBufAdapters []*stateBankPostBufAdapter

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage

	state                cacheState
	inFlightTransactions []*transactionState
	evictingList         map[uint64]bool
}

// --- NamedHookable delegation ---

func (m *pipelineMW) Name() string {
	return m.comp.Name()
}

func (m *pipelineMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *pipelineMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *pipelineMW) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *pipelineMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
	return m.comp.GetSpec()
}

// findPort resolves an address to a remote port using data from Spec.
func (m *pipelineMW) findPort(address uint64) sim.RemotePort {
	spec := m.comp.GetSpec()

	switch spec.AddressMapperType {
	case "single":
		return sim.RemotePort(spec.RemotePortNames[0])
	case "interleaved":
		n := uint64(len(spec.RemotePortNames))
		idx := address / spec.InterleavingSize % n
		return sim.RemotePort(spec.RemotePortNames[idx])
	}

	panic("unknown address mapper type: " + spec.AddressMapperType)
}

// Tick updates the internal states of the Cache pipeline.
func (m *pipelineMW) Tick() bool {
	m.updateAdapterPointers()

	madeProgress := false

	if m.state != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

// syncForTest synchronizes curState from the next state buffer and updates
// adapter read pointers. This is only needed in tests where state is set up
// via GetNextState() without going through the Component.Tick() cycle.
func (m *pipelineMW) syncForTest() {
	next := m.comp.GetNextState()
	m.comp.SetState(*next)
	m.curState = m.comp.GetState()
	next = m.comp.GetNextState()

	// Update adapter read pointers to curState, write pointers to next
	if m.dirStageBuffer != nil {
		m.dirStageBuffer.readItems = &m.curState.DirStageBufIndices
		m.dirStageBuffer.writeItems = &next.DirStageBufIndices
	}
	for i := range m.dirToBankBuffers {
		if m.dirToBankBuffers[i] != nil {
			m.dirToBankBuffers[i].readItems = &m.curState.DirToBankBufIndices[i].Indices
			m.dirToBankBuffers[i].writeItems = &next.DirToBankBufIndices[i].Indices
		}
	}
	for i := range m.writeBufferToBankBuffers {
		if m.writeBufferToBankBuffers[i] != nil {
			m.writeBufferToBankBuffers[i].readItems = &m.curState.WriteBufferToBankBufIndices[i].Indices
			m.writeBufferToBankBuffers[i].writeItems = &next.WriteBufferToBankBufIndices[i].Indices
		}
	}
	if m.mshrStageBuffer != nil {
		m.mshrStageBuffer.readItems = &m.curState.MSHRStageBufEntries
		m.mshrStageBuffer.writeItems = &next.MSHRStageBufEntries
	}
	if m.writeBufferBuffer != nil {
		m.writeBufferBuffer.readItems = &m.curState.WriteBufferBufIndices
		m.writeBufferBuffer.writeItems = &next.WriteBufferBufIndices
	}
	if m.dirPostBufAdapter != nil {
		m.dirPostBufAdapter.readItems = &m.curState.DirPostPipelineBufIndices
		m.dirPostBufAdapter.writeItems = &next.DirPostPipelineBufIndices
	}
	for i := range m.bankPostBufAdapters {
		if m.bankPostBufAdapters[i] != nil {
			m.bankPostBufAdapters[i].readItems = &m.curState.BankPostPipelineBufIndices[i].Indices
			m.bankPostBufAdapters[i].writeItems = &next.BankPostPipelineBufIndices[i].Indices
		}
	}
}

func (m *pipelineMW) updateAdapterPointers() {
	m.curState = m.comp.GetState()
	next := m.comp.GetNextState()

	// Dir stage buffer adapter
	m.dirStageBuffer.readItems = &m.curState.DirStageBufIndices
	m.dirStageBuffer.writeItems = &next.DirStageBufIndices

	// Dir to bank buffer adapters
	for i := range m.dirToBankBuffers {
		m.dirToBankBuffers[i].readItems = &m.curState.DirToBankBufIndices[i].Indices
		m.dirToBankBuffers[i].writeItems = &next.DirToBankBufIndices[i].Indices
	}

	// Write buffer to bank buffer adapters
	for i := range m.writeBufferToBankBuffers {
		m.writeBufferToBankBuffers[i].readItems = &m.curState.WriteBufferToBankBufIndices[i].Indices
		m.writeBufferToBankBuffers[i].writeItems = &next.WriteBufferToBankBufIndices[i].Indices
	}

	// MSHR stage buffer adapter
	m.mshrStageBuffer.readItems = &m.curState.MSHRStageBufEntries
	m.mshrStageBuffer.writeItems = &next.MSHRStageBufEntries

	// Write buffer buffer adapter
	m.writeBufferBuffer.readItems = &m.curState.WriteBufferBufIndices
	m.writeBufferBuffer.writeItems = &next.WriteBufferBufIndices

	// Dir post pipeline buf adapter
	m.dirPostBufAdapter.readItems = &m.curState.DirPostPipelineBufIndices
	m.dirPostBufAdapter.writeItems = &next.DirPostPipelineBufIndices

	// Bank post pipeline buf adapters
	for i := range m.bankPostBufAdapters {
		m.bankPostBufAdapters[i].readItems = &m.curState.BankPostPipelineBufIndices[i].Indices
		m.bankPostBufAdapters[i].writeItems = &next.BankPostPipelineBufIndices[i].Indices
	}
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

	m.dirStage.Reset()

	for _, bs := range m.bankStages {
		bs.Reset()
	}

	m.mshrStage.Reset()
	m.writeBuffer.Reset()

	clearPort(m.topPort)

	m.inFlightTransactions = nil
}

// GetState converts runtime mutable data into a serializable State.
func (m *pipelineMW) GetState() State {
	next := m.comp.GetNextState()

	lookup := buildTransIndex(m.inFlightTransactions)
	next.Transactions = snapshotAllTransactions(
		m.inFlightTransactions, lookup)
	next.CacheState = int(m.state)
	next.EvictingList = snapshotEvictingList(m.evictingList)

	// Bank counters
	for i, bs := range m.bankStages {
		next.BankInflightTransCounts[i] = bs.inflightTransCount
		next.BankDownwardInflightTransCounts[i] = bs.downwardInflightTransCount
	}

	// Write buffer stage
	next.PendingEvictionIndices = snapshotTransList(m.writeBuffer.pendingEvictions, lookup)
	next.InflightFetchIndices = snapshotTransList(m.writeBuffer.inflightFetch, lookup)
	next.InflightEvictionIndices = snapshotTransList(m.writeBuffer.inflightEviction, lookup)

	// MSHR stage
	next.HasProcessingMSHREntry, next.ProcessingMSHREntryIdx =
		snapshotMSHRStageEntry(m.mshrStage, lookup)

	return *next
}

// SetState restores runtime mutable data from a serializable State.
func (m *pipelineMW) SetState(state State) {
	m.comp.SetState(state)

	m.state = cacheState(state.CacheState)

	allTrans := restoreAllTransactions(state.Transactions)
	m.inFlightTransactions = allTrans

	// Evicting list
	m.evictingList = make(map[uint64]bool)
	maps.Copy(m.evictingList, state.EvictingList)

	// Bank counters
	for i, bs := range m.bankStages {
		if i < len(state.BankInflightTransCounts) {
			bs.inflightTransCount = state.BankInflightTransCounts[i]
		}
		if i < len(state.BankDownwardInflightTransCounts) {
			bs.downwardInflightTransCount =
				state.BankDownwardInflightTransCounts[i]
		}
	}

	// Write buffer stage
	m.writeBuffer.pendingEvictions = restoreTransList(
		state.PendingEvictionIndices, allTrans)
	m.writeBuffer.inflightFetch = restoreTransList(
		state.InflightFetchIndices, allTrans)
	m.writeBuffer.inflightEviction = restoreTransList(
		state.InflightEvictionIndices, allTrans)

	// MSHR stage
	m.mshrStage.hasProcessingTrans = state.HasProcessingMSHREntry
	if state.HasProcessingMSHREntry &&
		state.ProcessingMSHREntryIdx >= 0 &&
		state.ProcessingMSHREntryIdx < len(allTrans) {
		trans := allTrans[state.ProcessingMSHREntryIdx]
		m.mshrStage.processingTrans = trans
		m.mshrStage.processingTransList = trans.mshrTransactions
		m.mshrStage.processingData = trans.mshrData
	}
}

// controlMW runs the flusher (flush/invalidate from controlPort,
// controls cache state).
type controlMW struct {
	comp    *modeling.Component[Spec, State]
	flusher *flusher
}

// --- NamedHookable delegation ---

func (m *controlMW) Name() string {
	return m.comp.Name()
}

func (m *controlMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *controlMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *controlMW) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *controlMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

// Tick runs the flusher.
func (m *controlMW) Tick() bool {
	return m.flusher.Tick()
}

// GetState snapshots flusher state into the next state buffer.
func (m *controlMW) GetState() State {
	next := m.comp.GetNextState()

	next.FlusherBlockToEvictRefs, next.HasProcessingFlush,
		next.ProcessingFlush = snapshotFlusherState(m.flusher)

	return *next
}

// SetState restores flusher state from a serializable State.
func (m *controlMW) SetState(state State) {
	m.flusher.blockToEvict = make([]blockRef, len(state.FlusherBlockToEvictRefs))
	copy(m.flusher.blockToEvict, state.FlusherBlockToEvictRefs)
	m.flusher.processingFlush = nil

	if state.HasProcessingFlush {
		m.flusher.processingFlush = &cache.FlushReq{
			MsgMeta:                 state.ProcessingFlush.MsgMeta,
			InvalidateAllCachelines: state.ProcessingFlush.InvalidateAllCachelines,
			DiscardInflight:         state.ProcessingFlush.DiscardInflight,
			PauseAfterFlushing:      state.ProcessingFlush.PauseAfterFlushing,
		}
	}
}
