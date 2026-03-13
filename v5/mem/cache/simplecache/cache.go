package simplecache

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

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

	transactions             []*transactionState
	postCoalesceTransactions []*transactionState

	isPaused bool
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
	madeProgress := false

	if !m.isPaused {
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

// GetState converts runtime mutable data into a serializable State.
func (m *pipelineMW) GetState() State {
	next := m.comp.GetNextState()

	// Compact nil entries from postCoalesceTransactions before snapshot.
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
// postCoalesceTransactions and remaps all indices in State buffers/pipelines.
func (m *pipelineMW) compactPostCoalesceTransactions(next *State) {
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

	next.DirBuf.Elements = remapIndices(next.DirBuf.Elements)
	for i := range next.BankBufs {
		next.BankBufs[i].Elements = remapIndices(next.BankBufs[i].Elements)
	}
	next.DirPostBuf.Elements = remapIndices(next.DirPostBuf.Elements)
	for i := range next.BankPostBufs {
		next.BankPostBufs[i].Elements = remapIndices(next.BankPostBufs[i].Elements)
	}
	for i := range next.DirPipeline.Stages {
		if newIdx, ok := remap[next.DirPipeline.Stages[i].Item]; ok {
			next.DirPipeline.Stages[i].Item = newIdx
		}
	}
	for i := range next.BankPipelines {
		for j := range next.BankPipelines[i].Stages {
			if newIdx, ok := remap[next.BankPipelines[i].Stages[j].Item]; ok {
				next.BankPipelines[i].Stages[j].Item = newIdx
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
func (m *pipelineMW) SetState(state State) {
	m.comp.SetState(state)

	// Restore transactions from state
	trans, postCoalesce := restoreAllTransactions(
		state.Transactions, state.NumTransactions)
	m.transactions = trans
	m.postCoalesceTransactions = postCoalesce
	m.isPaused = state.IsPaused
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
