package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
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

	// Address mapper configuration (inlined from interface)
	AddressMapperType string   `json:"address_mapper_type"`
	RemotePortNames   []string `json:"remote_port_names"`
	InterleavingSize  uint64   `json:"interleaving_size"`
}

// State contains mutable runtime data for the writearound cache.
type State struct {
	DirectoryState             cache.DirectoryState    `json:"directory_state"`
	MSHRState                  cache.MSHRState         `json:"mshr_state"`
	Transactions               []transactionSnapshot   `json:"transactions"`
	NumTransactions            int                     `json:"num_transactions"`
	DirBufIndices              []int                   `json:"dir_buf_indices"`
	BankBufIndices             []bankBufState          `json:"bank_buf_indices"`
	DirPipelineStages          []dirPipelineStageState `json:"dir_pipeline_stages"`
	DirPostPipelineBufIndices  []int                   `json:"dir_post_pipeline_buf_indices"`
	BankPipelineStages         []bankPipelineState     `json:"bank_pipeline_stages"`
	BankPostPipelineBufIndices []bankPostBufState      `json:"bank_post_pipeline_buf_indices"`
	IsPaused                   bool                    `json:"is_paused"`
}

// middleware holds all non-serializable infrastructure for the writearound
// cache. It implements the Tick method and delegates NamedHookable to comp.
type middleware struct {
	comp *modeling.Component[Spec, State]

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	storage *mem.Storage

	// curState holds the A-buffer snapshot for the current tick.
	// Stored here so adapter read pointers remain valid for the tick duration.
	// In production, set by updateAdapterPointers() from comp.GetState().
	// In tests, set by syncForTest() from *comp.GetNextState().
	curState State

	// Thin buffer adapters (created once, pointers updated per-tick)
	dirBufAdapter       *stateTransBuffer
	bankBufAdapters     []*stateTransBuffer
	dirPostBufAdapter   *stateDirPostBufAdapter
	bankPostBufAdapters []*stateBankPostBufAdapter

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

// findPort resolves an address to a remote port using data from Spec.
func (m *middleware) findPort(address uint64) sim.RemotePort {
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

// Tick updates the state of the cache.
func (m *middleware) Tick() bool {
	m.updateAdapterPointers()

	madeProgress := false

	if !m.isPaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	madeProgress = m.controlStage.Tick() || madeProgress

	return madeProgress
}

// syncForTest synchronizes curState from the next state buffer and updates
// adapter read pointers. This is only needed in tests where state is set up
// via GetNextState() without going through the Component.Tick() cycle.
func (m *middleware) syncForTest() {
	next := m.comp.GetNextState()
	m.comp.SetState(*next)
	m.curState = m.comp.GetState()
	next = m.comp.GetNextState()

	// Update adapter read pointers to curState, write pointers to next
	if m.dirBufAdapter != nil {
		m.dirBufAdapter.readItems = &m.curState.DirBufIndices
		m.dirBufAdapter.writeItems = &next.DirBufIndices
	}
	for i := range m.bankBufAdapters {
		if m.bankBufAdapters[i] != nil {
			m.bankBufAdapters[i].readItems = &m.curState.BankBufIndices[i].Indices
			m.bankBufAdapters[i].writeItems = &next.BankBufIndices[i].Indices
		}
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

func (m *middleware) updateAdapterPointers() {
	m.curState = m.comp.GetState()
	next := m.comp.GetNextState()

	// Dir buf adapter
	m.dirBufAdapter.readItems = &m.curState.DirBufIndices
	m.dirBufAdapter.writeItems = &next.DirBufIndices

	// Bank buf adapters
	for i := range m.bankBufAdapters {
		m.bankBufAdapters[i].readItems = &m.curState.BankBufIndices[i].Indices
		m.bankBufAdapters[i].writeItems = &next.BankBufIndices[i].Indices
	}

	// Dir post pipeline buf adapter
	m.dirPostBufAdapter.readItems = &m.curState.DirPostPipelineBufIndices
	m.dirPostBufAdapter.writeItems = &next.DirPostPipelineBufIndices

	// Bank post pipeline buf adapters
	for i := range m.bankPostBufAdapters {
		m.bankPostBufAdapters[i].readItems = &m.curState.BankPostPipelineBufIndices[i].Indices
		m.bankPostBufAdapters[i].writeItems = &next.BankPostPipelineBufIndices[i].Indices
	}
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
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickParseBottomStage() bool {
	madeProgress := false

	spec := m.comp.GetSpec()
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
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.coalesceStage.Tick() || madeProgress
	}

	return madeProgress
}

// GetState converts runtime mutable data into a serializable State.
func (m *middleware) GetState() State {
	next := m.comp.GetNextState()

	// Compact nil entries from postCoalesceTransactions before snapshot.
	// During a tick, removeTransaction nils out entries to keep indices stable;
	// here we compact and remap all indices in State arrays.
	m.compactPostCoalesceTransactions(next)

	// Snapshot transactions into the state
	lookup := buildTransIndex(m.transactions, m.postCoalesceTransactions)
	next.Transactions = snapshotAllTransactions(
		m.transactions, m.postCoalesceTransactions, lookup)
	next.NumTransactions = len(m.transactions)
	next.IsPaused = m.isPaused

	return *next
}

// compactPostCoalesceTransactions removes nil entries from
// postCoalesceTransactions and remaps all indices in State arrays.
func (m *middleware) compactPostCoalesceTransactions(next *State) {
	old := m.postCoalesceTransactions
	remap := make(map[int]int)
	compacted := make([]*transactionState, 0, len(old))

	for i, t := range old {
		if t != nil {
			remap[i] = len(compacted)
			compacted = append(compacted, t)
		}
	}

	if len(compacted) == len(old) {
		return // nothing to compact
	}

	m.postCoalesceTransactions = compacted

	// Remap all index arrays in State
	remapIndices := func(indices []int) []int {
		result := make([]int, 0, len(indices))
		for _, idx := range indices {
			if newIdx, ok := remap[idx]; ok {
				result = append(result, newIdx)
			}
		}
		return result
	}

	next.DirBufIndices = remapIndices(next.DirBufIndices)
	for i := range next.BankBufIndices {
		next.BankBufIndices[i].Indices = remapIndices(next.BankBufIndices[i].Indices)
	}
	next.DirPostPipelineBufIndices = remapIndices(next.DirPostPipelineBufIndices)
	for i := range next.BankPostPipelineBufIndices {
		next.BankPostPipelineBufIndices[i].Indices = remapIndices(
			next.BankPostPipelineBufIndices[i].Indices)
	}
	for i := range next.DirPipelineStages {
		if newIdx, ok := remap[next.DirPipelineStages[i].TransIndex]; ok {
			next.DirPipelineStages[i].TransIndex = newIdx
		}
	}
	for i := range next.BankPipelineStages {
		for j := range next.BankPipelineStages[i].Stages {
			if newIdx, ok := remap[next.BankPipelineStages[i].Stages[j].TransIndex]; ok {
				next.BankPipelineStages[i].Stages[j].TransIndex = newIdx
			}
		}
	}
	// Also remap MSHR TransactionIndices
	for i := range next.MSHRState.Entries {
		entry := &next.MSHRState.Entries[i]
		if len(entry.TransactionIndices) > 0 {
			entry.TransactionIndices = remapIndices(entry.TransactionIndices)
		}
	}
}

// SetState restores runtime mutable data from a serializable State.
func (m *middleware) SetState(state State) {
	m.comp.SetState(state)

	// Restore transactions from state
	trans, postCoalesce := restoreAllTransactions(
		state.Transactions, state.NumTransactions)
	m.transactions = trans
	m.postCoalesceTransactions = postCoalesce
	m.isPaused = state.IsPaused
}
