package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
)

// Spec contains immutable configuration for the writethroughcache.
type Spec struct {
	Freq                  sim.Freq `json:"freq"`
	NumReqPerCycle        int      `json:"num_req_per_cycle"`
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

// State contains mutable runtime data for the writethroughcache.
type State struct {
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState      `json:"mshr_state"`

	// Transactions stores all transaction states directly as values.
	// The first NumTransactions entries are pre-coalesce transactions;
	// the remaining entries are post-coalesce transactions.
	Transactions    []transactionState `json:"transactions"`
	NumTransactions int                `json:"num_transactions"`

	DirBuf        stateutil.Buffer[int]     `json:"dir_buf"`
	BankBufs      []stateutil.Buffer[int]   `json:"bank_bufs"`
	DirPipeline   stateutil.Pipeline[int]   `json:"dir_pipeline"`
	DirPostBuf    stateutil.Buffer[int]     `json:"dir_post_buf"`
	BankPipelines []stateutil.Pipeline[int] `json:"bank_pipelines"`
	BankPostBufs  []stateutil.Buffer[int]   `json:"bank_post_bufs"`

	IsPaused bool `json:"is_paused"`
}

// postCoalesceTrans returns a pointer to the post-coalesce transaction
// at the given post-coalesce index (0-based relative to post-coalesce section).
func (s *State) postCoalesceTrans(idx int) *transactionState {
	return &s.Transactions[s.NumTransactions+idx]
}

// numPostCoalesce returns the number of post-coalesce transactions.
func (s *State) numPostCoalesce() int {
	return len(s.Transactions) - s.NumTransactions
}

// addPreCoalesceTrans appends a pre-coalesce transaction and returns its
// absolute index in State.Transactions.
func (s *State) addPreCoalesceTrans(t transactionState) int {
	postStart := s.NumTransactions
	// Insert before post-coalesce section
	s.Transactions = append(s.Transactions, transactionState{})
	copy(s.Transactions[postStart+1:], s.Transactions[postStart:len(s.Transactions)-1])
	s.Transactions[postStart] = t
	s.NumTransactions++
	return postStart
}

// addPostCoalesceTrans appends a post-coalesce transaction and returns its
// post-coalesce index (0-based).
func (s *State) addPostCoalesceTrans(t transactionState) int {
	s.Transactions = append(s.Transactions, t)
	return len(s.Transactions) - s.NumTransactions - 1
}

// pipelineMW holds all non-serializable infrastructure for the cache data
// pipeline. It implements the Tick method and delegates NamedHookable to comp.
type pipelineMW struct {
	comp *modeling.Component[Spec, State]

	topPort    sim.Port
	bottomPort sim.Port

	// legacyMapper is kept for backward compatibility with code that sets the
	// mapper's Port field after Build().
	legacyMapper mem.AddressToPortMapper

	writePolicy WritePolicy

	storage *mem.Storage

	coalesceStage    *coalescer
	directoryStage   *directory
	bankStages       []*bankStage
	parseBottomStage *bottomParser
	respondStage     *respondStage
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
	return m.comp.GetSpec()
}

// findPort resolves an address to a remote port using data from Spec.
// Falls back to legacyMapper when the Spec port names are empty.
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

	if m.legacyMapper != nil {
		return m.legacyMapper.Find(address)
	}

	panic("findPort: no valid address mapping for address; " +
		"Spec.AddressMapperType=" + spec.AddressMapperType)
}

// Tick updates the state of the cache pipeline.
func (m *pipelineMW) Tick() bool {
	next := m.comp.GetNextState()
	madeProgress := false

	if !next.IsPaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false
	madeProgress = m.tickRespondStage() || madeProgress
	madeProgress = m.tickParseBottomStage() || madeProgress
	madeProgress = m.tickBankStage() || madeProgress
	madeProgress = m.tickDirectoryStage() || madeProgress
	madeProgress = m.tickCoalesceState() || madeProgress

	return madeProgress
}

func (m *pipelineMW) tickRespondStage() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) tickParseBottomStage() bool {
	madeProgress := false

	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottomStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) tickBankStage() bool {
	madeProgress := false
	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) tickDirectoryStage() bool {
	return m.directoryStage.Tick()
}

func (m *pipelineMW) tickCoalesceState() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.coalesceStage.Tick() || madeProgress
	}

	return madeProgress
}

// controlMW runs the control stage (flush/invalidate/restart).
type controlMW struct {
	comp         *modeling.Component[Spec, State]
	controlStage *controlStage
}

// Tick runs the control stage.
func (m *controlMW) Tick() bool {
	return m.controlStage.Tick()
}
