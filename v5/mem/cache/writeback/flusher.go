package writeback

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// blockRef is a set+way pair referencing a block in the directory.
type blockRef struct {
	SetID int `json:"set_id"`
	WayID int `json:"way_id"`
}

type flusher struct {
	pipeline *pipelineMW
	ctrlPort sim.Port

	blockToEvict    []blockRef
	processingFlush *cache.FlushReq
}

func (f *flusher) Tick() bool {
	if f.processingFlush != nil && f.pipeline.state == cacheStatePreFlushing {
		return f.processPreFlushing()
	}

	madeProgress := false
	if f.processingFlush != nil && f.pipeline.state == cacheStateFlushing {
		madeProgress = f.finalizeFlushing() || madeProgress
		madeProgress = f.processFlush() || madeProgress

		return madeProgress
	}

	return f.extractFromPort()
}

func (f *flusher) processPreFlushing() bool {
	if f.existInflightTransaction() {
		return false
	}

	f.prepareBlockToFlushList()
	f.pipeline.state = cacheStateFlushing

	return true
}

func (f *flusher) existInflightTransaction() bool {
	for _, t := range f.pipeline.inFlightTransactions {
		if t != nil {
			return true
		}
	}
	return false
}

func (f *flusher) prepareBlockToFlushList() {
	next := f.pipeline.comp.GetNextState()
	for setID, set := range next.DirectoryState.Sets {
		for wayID, block := range set.Blocks {
			if block.ReadCount > 0 || block.IsLocked {
				panic("all the blocks should be unlocked before flushing")
			}

			if block.IsValid && block.IsDirty {
				f.blockToEvict = append(f.blockToEvict,
					blockRef{SetID: setID, WayID: wayID})
			}
		}
	}
}

func (f *flusher) processFlush() bool {
	if len(f.blockToEvict) == 0 {
		return false
	}

	next := f.pipeline.comp.GetNextState()
	spec := f.pipeline.comp.GetSpec()
	ref := f.blockToEvict[0]
	block := &next.DirectoryState.Sets[ref.SetID].Blocks[ref.WayID]
	bankNum := bankID(
		ref.SetID, ref.WayID,
		spec.WayAssociativity,
		len(next.DirToBankBufs))
	bankBuf := &next.DirToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans := &transactionState{
		flush:              f.processingFlush,
		hasVictim:          true,
		victimPID:          0,
		victimTag:          block.Tag,
		victimCacheAddress: block.CacheAddress,
		action:             bankEvict,
		evictingAddr:       block.Tag,
		evictingDirtyMask:  block.DirtyMask,
		blockSetID:         ref.SetID,
		blockWayID:         ref.WayID,
		hasBlock:           true,
	}

	f.pipeline.inFlightTransactions = append(f.pipeline.inFlightTransactions, trans)
	transIdx := len(f.pipeline.inFlightTransactions) - 1
	bankBuf.PushTyped(transIdx)

	f.blockToEvict = f.blockToEvict[1:]

	return true
}

func (f *flusher) extractFromPort() bool {
	msg := f.ctrlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *cache.FlushReq:
		return f.startProcessingFlush(msg)
	case *cache.RestartReq:
		return f.handleCacheRestart(msg)
	default:
		log.Panicf("Cannot process request of type %T", msg)
	}

	return true
}

func (f *flusher) startProcessingFlush(msg *cache.FlushReq) bool {
	f.processingFlush = msg
	if msg.DiscardInflight {
		f.pipeline.discardInflightTransactions()
	}

	f.pipeline.state = cacheStatePreFlushing
	f.ctrlPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, f.pipeline.comp)

	return true
}

func (f *flusher) handleCacheRestart(msg *cache.RestartReq) bool {
	if !f.ctrlPort.CanSend() {
		return false
	}

	clearPort(f.pipeline.topPort)
	clearPort(f.pipeline.bottomPort)

	f.pipeline.state = cacheStateRunning

	rsp := &cache.RestartRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = f.ctrlPort.AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "cache.RestartRsp"
	f.ctrlPort.Send(rsp)

	f.ctrlPort.RetrieveIncoming()

	return true
}

func (f *flusher) finalizeFlushing() bool {
	if len(f.blockToEvict) > 0 {
		return false
	}

	if !f.flushCompleted() {
		return false
	}

	if !f.ctrlPort.CanSend() {
		return false
	}

	spec := f.pipeline.comp.GetSpec()
	next := f.pipeline.comp.GetNextState()

	rsp := &cache.FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = f.ctrlPort.AsRemote()
	rsp.Dst = f.processingFlush.Src
	rsp.RspTo = f.processingFlush.ID
	rsp.TrafficClass = "cache.FlushRsp"
	f.ctrlPort.Send(rsp)

	// Reset MSHR and directory state
	next.MSHRState = cache.MSHRState{}
	blockSize := 1 << spec.Log2BlockSize
	cache.DirectoryReset(
		&next.DirectoryState,
		spec.NumSets,
		spec.WayAssociativity,
		blockSize,
	)

	if f.processingFlush.PauseAfterFlushing {
		f.pipeline.state = cacheStatePaused
	} else {
		f.pipeline.state = cacheStateRunning
	}

	tracing.TraceReqComplete(f.processingFlush, f.pipeline.comp)
	f.processingFlush = nil

	return true
}

func (f *flusher) flushCompleted() bool {
	next := f.pipeline.comp.GetNextState()

	for i := range next.DirToBankBufs {
		if next.DirToBankBufs[i].Size() > 0 {
			return false
		}
	}

	for _, b := range f.pipeline.bankStages {
		if b.inflightTransCount > 0 {
			return false
		}
	}

	if next.WriteBufferBuf.Size() > 0 {
		return false
	}

	if len(f.pipeline.writeBuffer.inflightFetch) > 0 ||
		len(f.pipeline.writeBuffer.inflightEviction) > 0 ||
		len(f.pipeline.writeBuffer.pendingEvictions) > 0 {
		return false
	}

	return true
}
