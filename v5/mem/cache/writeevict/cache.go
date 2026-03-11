package writeevict

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the writeevict cache.
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

// State contains mutable runtime data for the writeevict cache.
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

// middleware holds all non-serializable infrastructure for the writeevict
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

// SetAddressToPortMapper sets the finder that tells which remote port can serve
// the data on a certain address.
func (m *middleware) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	m.addressToPortMapper = lmf
}

// Tick updates the state of the cache.
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

// --- State snapshot/restore ---

func (m *middleware) snapshotState() State {
	lookup := buildTransIndex(
		m.transactions, m.postCoalesceTransactions)

	s := State{
		IsPaused:        m.isPaused,
		NumTransactions: len(m.transactions),
	}

	// DirectoryState and MSHRState are already the canonical state.
	// Deep copy them for the snapshot.
	s.DirectoryState = deepCopyDirectoryState(m.directoryState)
	s.MSHRState = deepCopyMSHRState(m.mshrState)

	s.Transactions = snapshotAllTransactions(
		m.transactions, m.postCoalesceTransactions, lookup)
	s.DirBufIndices = snapshotDirBuf(m.dirBuf, lookup)
	s.BankBufIndices = snapshotBankBufs(m.bankBufs, lookup)
	s.DirPipelineStages = snapshotDirPipeline(
		m.directoryStage.pipeline, lookup)
	s.DirPostPipelineBufIndices = snapshotDirPostBuf(
		m.directoryStage.buf, lookup)
	s.BankPipelineStages = snapshotBankPipelines(
		m.bankStages, lookup)
	s.BankPostPipelineBufIndices = snapshotBankPostBufs(
		m.bankStages, lookup)

	return s
}

func (m *middleware) restoreFromState(s State) {
	m.isPaused = s.IsPaused

	m.directoryState = deepCopyDirectoryState(s.DirectoryState)
	m.mshrState = deepCopyMSHRState(s.MSHRState)

	trans, postCoalesce := restoreAllTransactions(s.Transactions, s.NumTransactions)
	m.transactions = trans
	m.postCoalesceTransactions = postCoalesce

	allTrans := make([]*transactionState, len(s.Transactions))
	copy(allTrans[:s.NumTransactions], trans)
	copy(allTrans[s.NumTransactions:], postCoalesce)

	restoreBuffersAndPipelines(m, s, allTrans)
}

func restoreBuffersAndPipelines(
	m *middleware,
	s State,
	allTrans []*transactionState,
) {
	restoreDirBuf(m.dirBuf, s.DirBufIndices, allTrans)
	restoreBankBufs(m.bankBufs, s.BankBufIndices, allTrans)
	restoreDirPipeline(
		m.directoryStage.pipeline, s.DirPipelineStages, allTrans)
	restoreDirPostBuf(
		m.directoryStage.buf, s.DirPostPipelineBufIndices, allTrans)
	restoreBankPipelines(m.bankStages, s.BankPipelineStages, allTrans)
	restoreBankPostBufs(
		m.bankStages, s.BankPostPipelineBufIndices, allTrans)
}

// GetState converts runtime mutable data into a serializable State.
func (m *middleware) GetState() State {
	state := m.snapshotState()
	m.comp.SetState(state)

	return state
}

// SetState restores runtime mutable data from a serializable State.
func (m *middleware) SetState(state State) {
	m.comp.SetState(state)
	m.restoreFromState(state)
}
