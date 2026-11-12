package writeback

import (
	"github.com/sarchlab/akita/v5/mem/internal/addrfilter"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// blockRef is a set+way pair referencing a block in the directory.
type blockRef struct {
	SetID int `json:"set_id"`
	WayID int `json:"way_id"`
}

type flusher struct {
	pipeline *pipelineMW
}

// ctrlPort returns the Control port.
func (f *flusher) ctrlPort() messaging.Port {
	return f.pipeline.comp.Ports.Control
}

func (f *flusher) Tick() bool {
	next := &f.pipeline.comp.State

	if next.Maintenance.Command != 0 || len(next.Maintenance.Deferred) > 0 {
		return false
	}

	if next.HasProcessingFlush && cacheState(next.CacheState) == cacheStatePreFlushing {
		return f.processPreFlushing()
	}

	madeProgress := false
	if next.HasProcessingFlush && cacheState(next.CacheState) == cacheStateFlushing {
		madeProgress = f.finalizeFlushing() || madeProgress
		madeProgress = issueFlushWriteback(f.pipeline.comp, next.ProcessingFlush.RecvTaskID, false) || madeProgress

		return madeProgress
	}

	// Control commands are processed serially: while a Drain is in progress the
	// flusher must not consume a queued Flush — it stays on the Control port
	// until the drain settles into paused, where it is handled fresh.
	if cacheState(next.CacheState) == cacheStateDraining {
		return false
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
	spec := f.pipeline.comp.Spec
	flush := &next.ProcessingFlush
	filter := addrfilter.New(flush.FilterAddresses, flush.FilterHasRange,
		flush.FilterRange, spec.Log2BlockSize)
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
			if !filter.Matches(block.Tag) {
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

func issueFlushWriteback(c *Comp, recvTaskID uint64, ownsLockedWay bool) bool {
	next := &c.State

	if len(next.FlusherBlockToEvictRefs) == 0 {
		return false
	}

	spec := c.Spec
	ref := next.FlusherBlockToEvictRefs[0]
	block := &next.DirectoryState.Sets[ref.SetID].Blocks[ref.WayID]
	bankNum := bankID(
		ref.SetID, ref.WayID,
		spec.NumWays,
		len(next.DirToBankBufs))
	bankBuf := &next.DirToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans := transactionState{
		HasFlush:           true,
		OwnsFlushLock:      ownsLockedWay,
		RecvTaskID:         recvTaskID,
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

	transIdx := next.allocTransaction(trans)
	bankBuf.Push(transIdx)

	next.FlusherBlockToEvictRefs = next.FlusherBlockToEvictRefs[1:]

	return true
}

// extractFromPort consumes only CmdFlush from the Control port. Every
// other verb (Pause, Drain, Enable, Reset, Invalidate) is owned by
// ctrlMiddleware and is left in the incoming queue.
func (f *flusher) extractFromPort() bool {
	msg, ok := f.ctrlPort().PeekIncoming()
	if !ok {
		return false
	}
	_, ok = msg.Payload.(memcontrolprotocol.Req)
	req := msg
	if !ok {
		return false
	}

	if req.Payload.(memcontrolprotocol.Req).Command != memcontrolprotocol.CmdFlush {
		return false
	}

	// ctrlMiddleware rejects a Flush whose addressing is not valid.
	if req.Payload.(memcontrolprotocol.Req).AddressingError() != "" {
		return false
	}

	// Running address-filtered Flush belongs to ctrlMiddleware.
	next := &f.pipeline.comp.State
	if cacheState(next.CacheState) != cacheStatePaused {
		if cacheState(next.CacheState) == cacheStateRunning && req.Payload.(memcontrolprotocol.Req).HasAddressFilter() {
			return false
		}
		return f.rejectFlush(req)
	}

	return f.startProcessingFlush(req)
}

// rejectFlush replies that Flush is illegal while the cache is Running.
func (f *flusher) rejectFlush(msg messaging.Msg) bool {
	if !f.ctrlPort().CanSend() {
		return false
	}

	f.ctrlPort().Send(makeCtrlRsp(f.pipeline.comp, memcontrolprotocol.CmdFlush,
		msg.Src, msg.ID, false, memcontrolprotocol.ErrMustBePausedOrDrained))
	f.ctrlPort().RetrieveIncoming()

	return true
}

func (f *flusher) startProcessingFlush(msg messaging.Msg) bool {
	next := &f.pipeline.comp.State

	next.HasProcessingFlush = true
	next.ProcessingFlush = flushReqState{
		Msg:             msg,
		FilterAddresses: msg.Payload.(memcontrolprotocol.Req).Addresses,
		FilterPID:       msg.Payload.(memcontrolprotocol.Req).PID,
		FilterHasRange:  msg.Payload.(memcontrolprotocol.Req).HasRange,
		FilterRange:     msg.Payload.(memcontrolprotocol.Req).Range,
	}

	next.CacheState = int(cacheStatePreFlushing)
	f.ctrlPort().RetrieveIncoming()

	next.ProcessingFlush.RecvTaskID = f.pipeline.comp.TaskTracker().StartReqIn(f.pipeline.comp, msg)

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

	if !f.ctrlPort().CanSend() {
		return false
	}

	rsp := messaging.Msg{Payload: memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdFlush, Success: true},
		ID:    f.pipeline.comp.NewID(),
		Src:   f.ctrlPort().AsRemote(),
		Dst:   next.ProcessingFlush.Msg.Src,
		RspTo: next.ProcessingFlush.Msg.ID,
	}

	f.ctrlPort().Send(rsp)

	// Per protocol, Flush leaves clean entries valid: only the blocks that
	// were written back are now clean (their dirty data is in backing
	// memory), while blocks outside the filter are untouched. Mark exactly
	// the flushed blocks clean; leave them valid.
	for _, ref := range next.ProcessingFlush.FlushedRefs {
		block := &next.DirectoryState.Sets[ref.SetID].Blocks[ref.WayID]
		block.IsDirty = false
		block.DirtyMask = nil
	}

	// The paused Flush path returns the cache to its previous state.
	next.CacheState = int(cacheStatePaused)

	f.pipeline.comp.TaskTracker().EndReqIn(next.ProcessingFlush.RecvTaskID)
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
