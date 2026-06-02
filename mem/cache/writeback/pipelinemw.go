package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

// pipelineMW holds all non-serializable infrastructure for the writeback
// cache pipeline. It implements the Tick method and delegates NamedHookable
// to comp. All mutable state is in comp.State.
type pipelineMW struct {
	comp *modeling.Component[Spec, State, Resources]

	topPort    messaging.Port
	bottomPort messaging.Port

	storage *mem.Storage

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
	return m.comp.Spec()
}

// findPort resolves an address to a remote port using data from Spec.
func (m *pipelineMW) findPort(address uint64) messaging.RemotePort {
	spec := m.comp.Spec()

	switch spec.AddressMapperType {
	case "single":
		if len(spec.RemotePortNames) > 0 {
			name := spec.RemotePortNames[0]
			if name != "" {
				return messaging.RemotePort(name)
			}
		}
	case "interleaved":
		if n := uint64(len(spec.RemotePortNames)); n > 0 {
			idx := address / spec.InterleavingSize % n
			name := spec.RemotePortNames[idx]
			if name != "" {
				return messaging.RemotePort(name)
			}
		}
	}

	panic("findPort: no valid address mapping for address; " +
		"Spec.AddressMapperType=" + spec.AddressMapperType)
}

// Tick updates the internal states of the Cache pipeline.
func (m *pipelineMW) Tick() bool {
	next := &m.comp.State
	madeProgress := false

	if cacheState(next.CacheState) != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false

	spec := m.comp.Spec()

	madeProgress = m.runStage(m.mshrStage, spec.NumReqPerCycle) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.dirStage, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.topParser, spec.NumReqPerCycle) || madeProgress

	return madeProgress
}

func (m *pipelineMW) runStage(stage modeling.Ticker, n int) bool {
	madeProgress := false
	for range n {
		madeProgress = stage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) discardInflightTransactions() {
	next := &m.comp.State

	for i := range next.DirectoryState.Sets {
		for j := range next.DirectoryState.Sets[i].Blocks {
			next.DirectoryState.Sets[i].Blocks[j].ReadCount = 0
			next.DirectoryState.Sets[i].Blocks[j].IsLocked = false
		}
	}

	// Clear all buffers and pipelines
	next.DirStageBuf.Clear()
	next.DirPipeline.Clear()
	next.DirPostPipelineBuf.Clear()
	for i := range next.DirToBankBufs {
		next.DirToBankBufs[i].Clear()
	}
	for i := range next.WriteBufferToBankBufs {
		next.WriteBufferToBankBufs[i].Clear()
	}
	for i := range next.BankPipelines {
		next.BankPipelines[i].Clear()
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
