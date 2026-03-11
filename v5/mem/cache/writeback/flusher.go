package writeback

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type flusher struct {
	cache *Comp

	blockToEvict    []*cache.Block
	processingFlush *cache.FlushReq
}

func (f *flusher) Tick() bool {
	if f.processingFlush != nil && f.cache.state == cacheStatePreFlushing {
		return f.processPreFlushing()
	}

	madeProgress := false
	if f.processingFlush != nil && f.cache.state == cacheStateFlushing {
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
	f.cache.state = cacheStateFlushing

	return true
}

func (f *flusher) existInflightTransaction() bool {
	return len(f.cache.inFlightTransactions) > 0
}

func (f *flusher) prepareBlockToFlushList() {
	sets := f.cache.directory.GetSets()
	for _, set := range sets {
		for _, block := range set.Blocks {
			if block.ReadCount > 0 || block.IsLocked {
				panic("all the blocks should be unlocked before flushing")
			}

			if block.IsValid && block.IsDirty {
				f.blockToEvict = append(f.blockToEvict, block)
			}
		}
	}
}

func (f *flusher) processFlush() bool {
	if len(f.blockToEvict) == 0 {
		return false
	}

	block := f.blockToEvict[0]
	bankNum := bankID(
		block,
		f.cache.directory.WayAssociativity(),
		len(f.cache.dirToBankBuffers))
	bankBuf := f.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans := &transaction{
		flush:             f.processingFlush,
		victim:            block,
		action:            bankEvict,
		evictingAddr:      block.Tag,
		evictingDirtyMask: block.DirtyMask,
	}
	bankBuf.Push(trans)

	f.blockToEvict = f.blockToEvict[1:]

	return true
}

func (f *flusher) extractFromPort() bool {
	msg := f.cache.controlPort.PeekIncoming()
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
		f.cache.discardInflightTransactions()
	}

	f.cache.state = cacheStatePreFlushing
	f.cache.controlPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, f.cache)

	return true
}

func (f *flusher) handleCacheRestart(msg *cache.RestartReq) bool {
	if !f.cache.controlPort.CanSend() {
		return false
	}

	clearPort(f.cache.topPort)
	clearPort(f.cache.bottomPort)

	f.cache.state = cacheStateRunning

	rsp := &cache.RestartRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = f.cache.controlPort.AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "cache.RestartRsp"
	f.cache.controlPort.Send(rsp)

	f.cache.controlPort.RetrieveIncoming()

	return true
}

func (f *flusher) finalizeFlushing() bool {
	if len(f.blockToEvict) > 0 {
		return false
	}

	if !f.flushCompleted() {
		return false
	}

	if !f.cache.controlPort.CanSend() {
		return false
	}

	rsp := &cache.FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = f.cache.controlPort.AsRemote()
	rsp.Dst = f.processingFlush.Src
	rsp.RspTo = f.processingFlush.ID
	rsp.TrafficClass = "cache.FlushRsp"
	f.cache.controlPort.Send(rsp)

	f.cache.mshr.Reset()
	f.cache.directory.Reset()

	if f.processingFlush.PauseAfterFlushing {
		f.cache.state = cacheStatePaused
	} else {
		f.cache.state = cacheStateRunning
	}

	tracing.TraceReqComplete(f.processingFlush, f.cache)
	f.processingFlush = nil

	return true
}

func (f *flusher) flushCompleted() bool {
	for _, b := range f.cache.dirToBankBuffers {
		if b.Size() > 0 {
			return false
		}
	}

	for _, b := range f.cache.bankStages {
		if b.inflightTransCount > 0 {
			return false
		}
	}

	if f.cache.writeBufferBuffer.Size() > 0 {
		return false
	}

	if len(f.cache.writeBuffer.inflightFetch) > 0 ||
		len(f.cache.writeBuffer.inflightEviction) > 0 ||
		len(f.cache.writeBuffer.pendingEvictions) > 0 {
		return false
	}

	return true
}
