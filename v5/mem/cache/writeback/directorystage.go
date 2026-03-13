package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type directoryStage struct {
	cache *pipelineMW
}

func (ds *directoryStage) Tick() (madeProgress bool) {
	madeProgress = ds.acceptNewTransaction() || madeProgress

	madeProgress = ds.tickPipeline() || madeProgress

	madeProgress = ds.processTransaction() || madeProgress

	return madeProgress
}

func (ds *directoryStage) tickPipeline() bool {
	next := ds.cache.comp.GetNextState()
	return next.DirPipeline.Tick(&next.DirPostPipelineBuf)
}

func (ds *directoryStage) processTransaction() bool {
	madeProgress := false
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if len(next.DirPostPipelineBuf.Elements) == 0 {
			break
		}

		idx := next.DirPostPipelineBuf.Elements[0]
		trans := ds.cache.inFlightTransactions[idx]

		addr := trans.accessReqAddress()
		cacheLineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

		if _, evicting := ds.cache.evictingList[cacheLineID]; evicting {
			break
		}

		if trans.HasRead {
			madeProgress = ds.doRead(trans) || madeProgress
			continue
		}

		madeProgress = ds.doWrite(trans) || madeProgress
	}

	return madeProgress
}

func (ds *directoryStage) acceptNewTransaction() bool {
	madeProgress := false
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		item := next.DirStageBuf.Peek()
		if item == nil {
			break
		}

		transIdx := item.(int)

		if spec.DirLatency == 0 {
			// Bypass pipeline: put directly in post-pipeline buffer
			if !next.DirPostPipelineBuf.CanPush() {
				break
			}
			next.DirPostPipelineBuf.PushTyped(transIdx)
			next.DirStageBuf.Pop()
			madeProgress = true
		} else {
			if !next.DirPipeline.CanAccept() {
				break
			}
			next.DirPipeline.Accept(transIdx)
			next.DirStageBuf.Pop()
			madeProgress = true
		}
	}

	return madeProgress
}

func (ds *directoryStage) Reset() {
	next := ds.cache.comp.GetNextState()
	next.DirPipeline.Stages = nil
	next.DirPostPipelineBuf.Clear()
	next.DirStageBuf.Clear()
}

func (ds *directoryStage) doRead(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(trans.ReadAddress, spec.Log2BlockSize)

	mshrIdx, found := cache.MSHRQuery(
		&next.MSHRState, trans.ReadPID, cachelineID)
	if found {
		return ds.handleReadMSHRHit(trans, mshrIdx)
	}

	setID, wayID, blockFound := cache.DirectoryLookup(
		&next.DirectoryState,
		spec.NumSets, 1<<spec.Log2BlockSize,
		trans.ReadPID, cachelineID)
	if blockFound {
		return ds.handleReadHit(trans, setID, wayID)
	}

	return ds.handleReadMiss(trans)
}

func (ds *directoryStage) handleReadMSHRHit(
	trans *transactionState,
	mshrIdx int,
) bool {
	next := ds.cache.comp.GetNextState()

	trans.MSHREntryIndex = mshrIdx
	trans.HasMSHREntry = true
	next.MSHRState.Entries[mshrIdx].TransactionIndices = append(
		next.MSHRState.Entries[mshrIdx].TransactionIndices,
		ds.findTransIndex(trans))

	ds.popDirPostBuf()

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(&trans.ReadMeta, ds.cache.comp),
		ds.cache.comp,
		"read-mshr-hit",
	)

	return true
}

func (ds *directoryStage) handleReadHit(
	trans *transactionState,
	setID, wayID int,
) bool {
	next := ds.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked {
		return false
	}

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(&trans.ReadMeta, ds.cache.comp),
		ds.cache.comp,
		"read-hit",
	)

	return ds.readFromBank(trans, setID, wayID)
}

func (ds *directoryStage) handleReadMiss(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	cacheLineID, _ := getCacheLineID(trans.ReadAddress, spec.Log2BlockSize)

	if cache.MSHRIsFull(&next.MSHRState, spec.NumMSHREntry) {
		return false
	}

	blockSize := 1 << spec.Log2BlockSize
	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState,
		spec.NumSets, blockSize,
		cacheLineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]

	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if ds.needEviction(victim) {
		ok := ds.evict(trans, victimSetID, victimWayID)
		if ok {
			tracing.AddTaskStep(
				tracing.MsgIDAtReceiver(&trans.ReadMeta, ds.cache.comp),
				ds.cache.comp,
				"read-miss",
			)
		}

		return ok
	}

	ok := ds.fetch(trans, victimSetID, victimWayID)
	if ok {
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(&trans.ReadMeta, ds.cache.comp),
			ds.cache.comp,
			"read-miss",
		)
	}

	return ok
}

func (ds *directoryStage) doWrite(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(trans.WriteAddress, spec.Log2BlockSize)

	mshrIdx, found := cache.MSHRQuery(
		&next.MSHRState, trans.WritePID, cachelineID)
	if found {
		ok := ds.doWriteMSHRHit(trans, mshrIdx)
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(&trans.WriteMeta, ds.cache.comp),
			ds.cache.comp,
			"write-mshr-hit",
		)

		return ok
	}

	setID, wayID, blockFound := cache.DirectoryLookup(
		&next.DirectoryState,
		spec.NumSets, 1<<spec.Log2BlockSize,
		trans.WritePID, cachelineID)
	if blockFound {
		ok := ds.doWriteHit(trans, setID, wayID)
		if ok {
			tracing.AddTaskStep(
				tracing.MsgIDAtReceiver(&trans.WriteMeta, ds.cache.comp),
				ds.cache.comp,
				"write-hit",
			)
		}

		return ok
	}

	ok := ds.doWriteMiss(trans)
	if ok {
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(&trans.WriteMeta, ds.cache.comp),
			ds.cache.comp,
			"write-miss",
		)
	}

	return ok
}

func (ds *directoryStage) doWriteMSHRHit(
	trans *transactionState,
	mshrIdx int,
) bool {
	next := ds.cache.comp.GetNextState()
	trans.MSHREntryIndex = mshrIdx
	trans.HasMSHREntry = true
	next.MSHRState.Entries[mshrIdx].TransactionIndices = append(
		next.MSHRState.Entries[mshrIdx].TransactionIndices,
		ds.findTransIndex(trans))

	ds.popDirPostBuf()

	return true
}

func (ds *directoryStage) doWriteHit(
	trans *transactionState,
	setID, wayID int,
) bool {
	next := ds.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	return ds.writeToBank(trans, setID, wayID)
}

func (ds *directoryStage) doWriteMiss(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	if ds.isWritingFullLine(trans, spec.Log2BlockSize) {
		return ds.writeFullLineMiss(trans)
	}

	return ds.writePartialLineMiss(trans)
}

func (ds *directoryStage) writeFullLineMiss(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(trans.WriteAddress, spec.Log2BlockSize)

	blockSize := 1 << spec.Log2BlockSize
	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState,
		spec.NumSets, blockSize,
		cachelineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]

	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if ds.needEviction(victim) {
		return ds.evict(trans, victimSetID, victimWayID)
	}

	return ds.writeToBank(trans, victimSetID, victimWayID)
}

func (ds *directoryStage) writePartialLineMiss(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(trans.WriteAddress, spec.Log2BlockSize)

	if cache.MSHRIsFull(&next.MSHRState, spec.NumMSHREntry) {
		return false
	}

	blockSize := 1 << spec.Log2BlockSize
	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState,
		spec.NumSets, blockSize,
		cachelineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]

	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if ds.needEviction(victim) {
		return ds.evict(trans, victimSetID, victimWayID)
	}

	return ds.fetch(trans, victimSetID, victimWayID)
}

func (ds *directoryStage) readFromBank(
	trans *transactionState,
	setID, wayID int,
) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	numBanks := len(next.DirToBankBufs)
	bank := bankID(setID, wayID, spec.WayAssociativity, numBanks)
	bankBuf := &next.DirToBankBufs[bank]

	if !bankBuf.CanPush() {
		return false
	}

	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.ReadCount++
	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true
	trans.Action = bankReadHit

	transIdx := ds.findTransIndex(trans)
	ds.popDirPostBuf()
	bankBuf.PushTyped(transIdx)

	return true
}

func (ds *directoryStage) writeToBank(
	trans *transactionState,
	setID, wayID int,
) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	numBanks := len(next.DirToBankBufs)
	bank := bankID(setID, wayID, spec.WayAssociativity, numBanks)
	bankBuf := &next.DirToBankBufs[bank]

	if !bankBuf.CanPush() {
		return false
	}

	addr := trans.WriteAddress
	cachelineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.IsLocked = true
	block.Tag = cachelineID
	block.IsValid = true
	block.PID = uint32(trans.WritePID)
	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true
	trans.Action = bankWriteHit

	transIdx := ds.findTransIndex(trans)
	ds.popDirPostBuf()
	bankBuf.PushTyped(transIdx)

	return true
}

func (ds *directoryStage) evict(
	trans *transactionState,
	victimSetID, victimWayID int,
) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	bankNum := bankID(victimSetID, victimWayID,
		spec.WayAssociativity, len(next.DirToBankBufs))
	bankBuf := &next.DirToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	var (
		addr uint64
		pid  vm.PID
	)

	if trans.HasRead {
		addr = trans.ReadAddress
		pid = trans.ReadPID
	} else {
		addr = trans.WriteAddress
		pid = trans.WritePID
	}

	cacheLineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

	ds.updateTransForEviction(trans, victimSetID, victimWayID, pid, cacheLineID)
	ds.updateVictimBlockMetaData(victimSetID, victimWayID, cacheLineID, pid)

	transIdx := ds.findTransIndex(trans)
	ds.popDirPostBuf()
	bankBuf.PushTyped(transIdx)

	ds.cache.evictingList[trans.VictimTag] = true

	return true
}

func (ds *directoryStage) updateVictimBlockMetaData(
	setID, wayID int,
	cacheLineID uint64,
	pid vm.PID,
) {
	next := ds.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.Tag = cacheLineID
	block.PID = uint32(pid)
	block.IsLocked = true
	block.IsDirty = false
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)
}

func (ds *directoryStage) updateTransForEviction(
	trans *transactionState,
	victimSetID, victimWayID int,
	pid vm.PID,
	cacheLineID uint64,
) {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]

	trans.Action = bankEvictAndFetch
	trans.HasVictim = true
	trans.VictimPID = vm.PID(victim.PID)
	trans.VictimTag = victim.Tag
	trans.VictimCacheAddress = victim.CacheAddress
	if victim.DirtyMask != nil {
		trans.VictimDirtyMask = make([]bool, len(victim.DirtyMask))
		copy(trans.VictimDirtyMask, victim.DirtyMask)
	}

	trans.BlockSetID = victimSetID
	trans.BlockWayID = victimWayID
	trans.HasBlock = true
	trans.EvictingPID = trans.VictimPID
	trans.EvictingAddr = trans.VictimTag
	trans.EvictingDirtyMask = trans.VictimDirtyMask

	if ds.evictionNeedFetch(trans, spec.Log2BlockSize) {
		mshrIdx := cache.MSHRAdd(
			&next.MSHRState, spec.NumMSHREntry,
			pid, cacheLineID)
		entry := &next.MSHRState.Entries[mshrIdx]
		entry.BlockSetID = victimSetID
		entry.BlockWayID = victimWayID
		entry.HasBlock = true
		entry.TransactionIndices = append(
			entry.TransactionIndices, ds.findTransIndex(trans))
		trans.MSHREntryIndex = mshrIdx
		trans.HasMSHREntry = true
		trans.FetchPID = pid
		trans.FetchAddress = cacheLineID
		trans.Action = bankEvictAndFetch
	} else {
		trans.Action = bankEvictAndWrite
	}
}

func (ds *directoryStage) evictionNeedFetch(t *transactionState, log2BlockSize uint64) bool {
	if !t.HasWrite {
		return true
	}

	if ds.isWritingFullLine(t, log2BlockSize) {
		return false
	}

	return true
}

func (ds *directoryStage) fetch(
	trans *transactionState,
	setID, wayID int,
) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()

	addr, pid, reqMeta := ds.transAddrPIDReqMeta(trans)
	cacheLineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

	bankNum := bankID(setID, wayID,
		spec.WayAssociativity, len(next.DirToBankBufs))
	bankBuf := &next.DirToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	mshrIdx := cache.MSHRAdd(
		&next.MSHRState, spec.NumMSHREntry,
		pid, cacheLineID)
	trans.MSHREntryIndex = mshrIdx
	trans.HasMSHREntry = true
	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true

	ds.updateBlockForFetch(next, setID, wayID, cacheLineID, pid)

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(reqMeta, ds.cache.comp),
		ds.cache.comp,
		fmt.Sprintf("add-mshr-entry-0x%x-0x%x",
			next.MSHRState.Entries[mshrIdx].Address,
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag),
	)

	ds.popDirPostBuf()

	trans.Action = writeBufferFetch
	trans.FetchPID = pid
	trans.FetchAddress = cacheLineID
	transIdx := ds.findTransIndex(trans)
	bankBuf.PushTyped(transIdx)

	ds.addMSHREntryBlock(next, mshrIdx, setID, wayID, trans)

	return true
}

func (ds *directoryStage) transAddrPIDReqMeta(
	trans *transactionState,
) (uint64, vm.PID, sim.Msg) {
	if trans.HasRead {
		return trans.ReadAddress, trans.ReadPID, &trans.ReadMeta
	}

	return trans.WriteAddress, trans.WritePID, &trans.WriteMeta
}

func (ds *directoryStage) updateBlockForFetch(
	next *State, setID, wayID int,
	cacheLineID uint64, pid vm.PID,
) {
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.IsLocked = true
	block.Tag = cacheLineID
	block.PID = uint32(pid)
	block.IsValid = true
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)
}

func (ds *directoryStage) addMSHREntryBlock(
	next *State, mshrIdx, setID, wayID int,
	trans *transactionState,
) {
	entry := &next.MSHRState.Entries[mshrIdx]
	entry.BlockSetID = setID
	entry.BlockWayID = wayID
	entry.HasBlock = true
	entry.TransactionIndices = append(
		entry.TransactionIndices, ds.findTransIndex(trans))
}

func (ds *directoryStage) isWritingFullLine(trans *transactionState, log2BlockSize uint64) bool {
	if len(trans.WriteData) != (1 << log2BlockSize) {
		return false
	}

	if trans.WriteDirtyMask != nil {
		for _, dirty := range trans.WriteDirtyMask {
			if !dirty {
				return false
			}
		}
	}

	return true
}

func (ds *directoryStage) needEviction(victim *cache.BlockState) bool {
	return victim.IsValid && victim.IsDirty
}

// findTransIndex finds the index of trans in the inFlightTransactions list.
func (ds *directoryStage) findTransIndex(trans *transactionState) int {
	for i, t := range ds.cache.inFlightTransactions {
		if t == trans {
			return i
		}
	}
	// Transaction might not be in inflight list (e.g. flush transactions)
	return -1
}

// popDirPostBuf removes the first element from the directory post-pipeline buffer.
func (ds *directoryStage) popDirPostBuf() {
	next := ds.cache.comp.GetNextState()
	if len(next.DirPostPipelineBuf.Elements) > 0 {
		next.DirPostPipelineBuf.Elements = next.DirPostPipelineBuf.Elements[1:]
	}
}
