package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// blockRef is a set+way pair referencing a block in the directory.
	"github.com/sarchlab/akita/v5/messaging"
)

type blockRef struct {
	SetID int `json:"set_id"`
	WayID int `json:"way_id"`
}

type flusher struct {
	pipeline *pipelineMW
	ctrlPort messaging.Port
}

func (f *flusher) Tick() bool {
	next := &f.pipeline.comp.State

	if next.HasProcessingFlush && cacheState(next.CacheState) == cacheStatePreFlushing {
		return f.processPreFlushing()
	}

	madeProgress := false
	if next.HasProcessingFlush && cacheState(next.CacheState) == cacheStateFlushing {
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
	next := &f.pipeline.comp.State
	next.CacheState = int(cacheStateFlushing)

	return true
}

func (f *flusher) existInflightTransaction() bool {
	next := &f.pipeline.comp.State
	for _, t := range next.Transactions {
		if !t.Removed {
			return true
		}
	}
	return false
}

func (f *flusher) prepareBlockToFlushList() {
	next := &f.pipeline.comp.State
	spec := f.pipeline.comp.Spec()
	blockSize := uint64(1) << spec.Log2BlockSize

	matchAddr := make(map[uint64]bool, len(next.ProcessingFlush.FilterAddresses))
	for _, a := range next.ProcessingFlush.FilterAddresses {
		matchAddr[a/blockSize*blockSize] = true
	}
	filterPID := next.ProcessingFlush.FilterPID

	for setID, set := range next.DirectoryState.Sets {
		for wayID, block := range set.Blocks {
			if block.ReadCount > 0 || block.IsLocked {
				panic("all the blocks should be unlocked before flushing")
			}

			if !block.IsValid || !block.IsDirty {
				continue
			}
			if filterPID != 0 && vm.PID(block.PID) != filterPID {
				continue
			}
			if len(matchAddr) > 0 && !matchAddr[block.Tag] {
				continue
			}

			ref := blockRef{SetID: setID, WayID: wayID}
			next.FlusherBlockToEvictRefs = append(
				next.FlusherBlockToEvictRefs, ref)
			next.ProcessingFlush.FlushedRefs = append(
				next.ProcessingFlush.FlushedRefs, ref)
		}
	}
}

func (f *flusher) processFlush() bool {
	next := &f.pipeline.comp.State

	if len(next.FlusherBlockToEvictRefs) == 0 {
		return false
	}

	spec := f.pipeline.comp.Spec()
	ref := next.FlusherBlockToEvictRefs[0]
	block := &next.DirectoryState.Sets[ref.SetID].Blocks[ref.WayID]
	bankNum := bankID(
		ref.SetID, ref.WayID,
		spec.WayAssociativity,
		len(next.DirToBankBufs))
	bankBuf := &next.DirToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans := transactionState{
		HasFlush:           true,
		FlushMeta:          next.ProcessingFlush.MsgMeta,
		HasVictim:          true,
		VictimPID:          vm.PID(block.PID),
		VictimTag:          block.Tag,
		VictimCacheAddress: block.CacheAddress,
		Action:             bankEvict,
		EvictingPID:        vm.PID(block.PID),
		EvictingAddr:       block.Tag,
		EvictingDirtyMask:  block.DirtyMask,
		BlockSetID:         ref.SetID,
		BlockWayID:         ref.WayID,
		HasBlock:           true,
	}

	next.Transactions = append(next.Transactions, trans)
	transIdx := len(next.Transactions) - 1
	bankBuf.PushTyped(transIdx)

	next.FlusherBlockToEvictRefs = next.FlusherBlockToEvictRefs[1:]

	return true
}

// extractFromPort consumes only CmdFlush from the Control port. Every
// other verb (Pause, Drain, Enable, Reset, Invalidate) is owned by
// ctrlMiddleware and is left in the incoming queue.
func (f *flusher) extractFromPort() bool {
	msg := f.ctrlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(*mem.ControlReq)
	if !ok {
		return false
	}

	if req.Command != mem.CmdFlush {
		return false
	}

	// Flush is a conditional verb: it is only legal once the cache is
	// paused (Drain lands the cache in paused too). Issued while Running it
	// is rejected with ErrMustBePausedOrDrained.
	next := &f.pipeline.comp.State
	if cacheState(next.CacheState) != cacheStatePaused {
		return f.rejectFlush(req)
	}

	return f.startProcessingFlush(req)
}

// rejectFlush replies that Flush is illegal while the cache is Running.
func (f *flusher) rejectFlush(msg *mem.ControlReq) bool {
	if !f.ctrlPort.CanSend() {
		return false
	}

	f.ctrlPort.Send(makeCtrlRsp(f.ctrlPort, mem.CmdFlush,
		msg.Src, msg.ID, false, control.ErrMustBePausedOrDrained))
	f.ctrlPort.RetrieveIncoming()

	return true
}

func (f *flusher) startProcessingFlush(msg *mem.ControlReq) bool {
	next := &f.pipeline.comp.State

	next.HasProcessingFlush = true
	next.ProcessingFlush = flushReqState{
		MsgMeta:         msg.MsgMeta,
		FilterAddresses: msg.Addresses,
		FilterPID:       msg.PID,
	}

	next.CacheState = int(cacheStatePreFlushing)
	f.ctrlPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, f.pipeline.comp)

	return true
}

func (f *flusher) finalizeFlushing() bool {
	next := &f.pipeline.comp.State

	if len(next.FlusherBlockToEvictRefs) > 0 {
		return false
	}

	if !f.flushCompleted() {
		return false
	}

	if !f.ctrlPort.CanSend() {
		return false
	}

	rsp := &mem.ControlRsp{Command: mem.CmdFlush, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = f.ctrlPort.AsRemote()
	rsp.Dst = next.ProcessingFlush.MsgMeta.Src
	rsp.RspTo = next.ProcessingFlush.MsgMeta.ID
	rsp.TrafficClass = "mem.ControlRsp"
	f.ctrlPort.Send(rsp)

	// Per protocol, Flush leaves clean entries valid: only the blocks that
	// were written back are now clean (their dirty data is in backing
	// memory), while blocks outside the filter are untouched. Mark exactly
	// the flushed blocks clean; leave them valid.
	for _, ref := range next.ProcessingFlush.FlushedRefs {
		block := &next.DirectoryState.Sets[ref.SetID].Blocks[ref.WayID]
		block.IsDirty = false
		block.DirtyMask = nil
	}

	// Flush is only legal from paused, and returns the cache to paused.
	next.CacheState = int(cacheStatePaused)

	tracing.TraceReqComplete(&next.ProcessingFlush.MsgMeta, f.pipeline.comp)
	next.HasProcessingFlush = false
	next.ProcessingFlush = flushReqState{}

	return true
}

func (f *flusher) flushCompleted() bool {
	next := &f.pipeline.comp.State

	for i := range next.DirToBankBufs {
		if next.DirToBankBufs[i].Size() > 0 {
			return false
		}
	}

	for i := range next.BankInflightTransCounts {
		if next.BankInflightTransCounts[i] > 0 {
			return false
		}
	}

	if next.WriteBufferBuf.Size() > 0 {
		return false
	}

	if len(next.InflightFetchIndices) > 0 ||
		len(next.InflightEvictionIndices) > 0 ||
		len(next.PendingEvictionIndices) > 0 {
		return false
	}

	return true
}
