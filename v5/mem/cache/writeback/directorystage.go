package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type dirPipelineItem struct {
	trans *transactionState
}

func (i dirPipelineItem) TaskID() string {
	return i.trans.id + "_dir_pipeline"
}

type directoryStage struct {
	cache *middleware
}

func (ds *directoryStage) Tick() (madeProgress bool) {
	madeProgress = ds.acceptNewTransaction() || madeProgress

	madeProgress = ds.tickPipeline() || madeProgress

	madeProgress = ds.processTransaction() || madeProgress

	return madeProgress
}

func (ds *directoryStage) tickPipeline() bool {
	next := ds.cache.comp.GetNextState()
	spec := ds.cache.comp.GetSpec()
	return dirPipelineTick(
		&next.DirPipelineStages,
		&next.DirPostPipelineBufIndices,
		spec.NumReqPerCycle,
		spec.DirLatency,
	)
}

func (ds *directoryStage) processTransaction() bool {
	madeProgress := false
	spec := ds.cache.comp.GetSpec()
	// We use next because processTransaction is called in a loop and
	// popDirPostBuf modifies next — subsequent iterations need to see removals.
	next := ds.cache.comp.GetNextState()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if len(next.DirPostPipelineBufIndices) == 0 {
			break
		}

		idx := next.DirPostPipelineBufIndices[0]
		trans := ds.cache.inFlightTransactions[idx]

		addr := trans.accessReq().GetAddress()
		cacheLineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

		if _, evicting := ds.cache.evictingList[cacheLineID]; evicting {
			break
		}

		if trans.read != nil {
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
	cur := ds.cache.comp.GetState()
	next := ds.cache.comp.GetNextState()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		item := ds.cache.dirStageBuffer.Peek()
		if item == nil {
			break
		}

		trans := item.(*transactionState)
		transIdx := ds.findTransIndex(trans)

		if spec.DirLatency == 0 {
			// Bypass pipeline: put directly in post-pipeline buffer
			if len(next.DirPostPipelineBufIndices) >= spec.NumReqPerCycle {
				break
			}
			next.DirPostPipelineBufIndices = append(
				next.DirPostPipelineBufIndices, transIdx)
			ds.cache.dirStageBuffer.Pop()
			madeProgress = true
		} else {
			if !dirPipelineCanAccept(cur.DirPipelineStages, spec.NumReqPerCycle) {
				break
			}
			dirPipelineAccept(&next.DirPipelineStages, spec.NumReqPerCycle, transIdx)
			ds.cache.dirStageBuffer.Pop()
			madeProgress = true
		}
	}

	return madeProgress
}

func (ds *directoryStage) Reset() {
	next := ds.cache.comp.GetNextState()
	next.DirPipelineStages = next.DirPipelineStages[:0]
	next.DirPostPipelineBufIndices = next.DirPostPipelineBufIndices[:0]
	ds.cache.dirStageBuffer.Clear()
}

func (ds *directoryStage) doRead(trans *transactionState) bool {
	read := trans.read
	spec := ds.cache.comp.GetSpec()
	// Use next for MSHRQuery and DirectoryLookup because multiple
	// transactions are processed per tick and need to see within-tick mutations.
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(read.Address, spec.Log2BlockSize)

	mshrIdx, found := cache.MSHRQuery(
		&next.MSHRState, read.PID, cachelineID)
	if found {
		return ds.handleReadMSHRHit(trans, mshrIdx)
	}

	setID, wayID, blockFound := cache.DirectoryLookup(
		&next.DirectoryState,
		spec.NumSets, 1<<spec.Log2BlockSize,
		read.PID, cachelineID)
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

	trans.mshrEntryIndex = mshrIdx
	trans.hasMSHREntry = true
	next.MSHRState.Entries[mshrIdx].TransactionIndices = append(
		next.MSHRState.Entries[mshrIdx].TransactionIndices,
		ds.findTransIndex(trans))

	ds.popDirPostBuf()

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(trans.read, ds.cache),
		ds.cache,
		"read-mshr-hit",
	)

	return true
}

func (ds *directoryStage) handleReadHit(
	trans *transactionState,
	setID, wayID int,
) bool {
	// Use next for block state check since within-tick mutations
	// (blocks locked by previous transactions) must be visible.
	next := ds.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked {
		return false
	}

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(trans.read, ds.cache),
		ds.cache,
		"read-hit",
	)

	return ds.readFromBank(trans, setID, wayID)
}

func (ds *directoryStage) handleReadMiss(trans *transactionState) bool {
	read := trans.read
	spec := ds.cache.comp.GetSpec()
	// Use next for MSHRIsFull and DirectoryFindVictim since within-tick
	// mutations must be visible.
	next := ds.cache.comp.GetNextState()
	cacheLineID, _ := getCacheLineID(read.Address, spec.Log2BlockSize)

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
				tracing.MsgIDAtReceiver(trans.read, ds.cache),
				ds.cache,
				"read-miss",
			)
		}

		return ok
	}

	ok := ds.fetch(trans, victimSetID, victimWayID)
	if ok {
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(trans.read, ds.cache),
			ds.cache,
			"read-miss",
		)
	}

	return ok
}

func (ds *directoryStage) doWrite(trans *transactionState) bool {
	write := trans.write
	spec := ds.cache.comp.GetSpec()
	// Use next for MSHRQuery and DirectoryLookup since within-tick
	// mutations must be visible.
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(write.Address, spec.Log2BlockSize)

	mshrIdx, found := cache.MSHRQuery(
		&next.MSHRState, write.PID, cachelineID)
	if found {
		ok := ds.doWriteMSHRHit(trans, mshrIdx)
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(trans.write, ds.cache),
			ds.cache,
			"write-mshr-hit",
		)

		return ok
	}

	setID, wayID, blockFound := cache.DirectoryLookup(
		&next.DirectoryState,
		spec.NumSets, 1<<spec.Log2BlockSize,
		write.PID, cachelineID)
	if blockFound {
		ok := ds.doWriteHit(trans, setID, wayID)
		if ok {
			tracing.AddTaskStep(
				tracing.MsgIDAtReceiver(trans.write, ds.cache),
				ds.cache,
				"write-hit",
			)
		}

		return ok
	}

	ok := ds.doWriteMiss(trans)
	if ok {
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(trans.write, ds.cache),
			ds.cache,
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
	trans.mshrEntryIndex = mshrIdx
	trans.hasMSHREntry = true
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
	// Use next for block state check since within-tick mutations must be visible.
	next := ds.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	return ds.writeToBank(trans, setID, wayID)
}

func (ds *directoryStage) doWriteMiss(trans *transactionState) bool {
	spec := ds.cache.comp.GetSpec()
	if ds.isWritingFullLine(trans.write, spec.Log2BlockSize) {
		return ds.writeFullLineMiss(trans)
	}

	return ds.writePartialLineMiss(trans)
}

func (ds *directoryStage) writeFullLineMiss(trans *transactionState) bool {
	write := trans.write
	spec := ds.cache.comp.GetSpec()
	// Use next for DirectoryFindVictim and block state checks since
	// within-tick mutations must be visible.
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(write.Address, spec.Log2BlockSize)

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
	write := trans.write
	spec := ds.cache.comp.GetSpec()
	// Use next for MSHRIsFull, DirectoryFindVictim, and block state checks
	// since within-tick mutations must be visible.
	next := ds.cache.comp.GetNextState()
	cachelineID, _ := getCacheLineID(write.Address, spec.Log2BlockSize)

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
	numBanks := len(ds.cache.dirToBankBuffers)
	bank := bankID(setID, wayID, spec.WayAssociativity, numBanks)
	bankBuf := ds.cache.dirToBankBuffers[bank]

	if !bankBuf.CanPush() {
		return false
	}

	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.ReadCount++
	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true
	trans.action = bankReadHit

	ds.popDirPostBuf()
	bankBuf.Push(trans)

	return true
}

func (ds *directoryStage) writeToBank(
	trans *transactionState,
	setID, wayID int,
) bool {
	spec := ds.cache.comp.GetSpec()
	next := ds.cache.comp.GetNextState()
	numBanks := len(ds.cache.dirToBankBuffers)
	bank := bankID(setID, wayID, spec.WayAssociativity, numBanks)
	bankBuf := ds.cache.dirToBankBuffers[bank]

	if !bankBuf.CanPush() {
		return false
	}

	write := trans.write
	addr := write.Address
	cachelineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.IsLocked = true
	block.Tag = cachelineID
	block.IsValid = true
	block.PID = uint32(write.PID)
	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true
	trans.action = bankWriteHit

	ds.popDirPostBuf()
	bankBuf.Push(trans)

	return true
}

func (ds *directoryStage) evict(
	trans *transactionState,
	victimSetID, victimWayID int,
) bool {
	spec := ds.cache.comp.GetSpec()
	bankNum := bankID(victimSetID, victimWayID,
		spec.WayAssociativity, len(ds.cache.dirToBankBuffers))
	bankBuf := ds.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	var (
		addr uint64
		pid  vm.PID
	)

	if trans.read != nil {
		addr = trans.read.Address
		pid = trans.read.PID
	} else {
		addr = trans.write.Address
		pid = trans.write.PID
	}

	cacheLineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

	ds.updateTransForEviction(trans, victimSetID, victimWayID, pid, cacheLineID)
	ds.updateVictimBlockMetaData(victimSetID, victimWayID, cacheLineID, pid)

	ds.popDirPostBuf()
	bankBuf.Push(trans)

	ds.cache.evictingList[trans.victimTag] = true

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
	// Read victim metadata from cur (frozen snapshot from previous tick)
	// since we need the original block metadata before any within-tick changes.
	cur := ds.cache.comp.GetState()
	next := ds.cache.comp.GetNextState()
	victim := &cur.DirectoryState.Sets[victimSetID].Blocks[victimWayID]

	trans.action = bankEvictAndFetch
	trans.hasVictim = true
	trans.victimPID = vm.PID(victim.PID)
	trans.victimTag = victim.Tag
	trans.victimCacheAddress = victim.CacheAddress
	if victim.DirtyMask != nil {
		trans.victimDirtyMask = make([]bool, len(victim.DirtyMask))
		copy(trans.victimDirtyMask, victim.DirtyMask)
	}

	trans.blockSetID = victimSetID
	trans.blockWayID = victimWayID
	trans.hasBlock = true
	trans.evictingPID = trans.victimPID
	trans.evictingAddr = trans.victimTag
	trans.evictingDirtyMask = trans.victimDirtyMask

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
		trans.mshrEntryIndex = mshrIdx
		trans.hasMSHREntry = true
		trans.fetchPID = pid
		trans.fetchAddress = cacheLineID
		trans.action = bankEvictAndFetch
	} else {
		trans.action = bankEvictAndWrite
	}
}

func (ds *directoryStage) evictionNeedFetch(t *transactionState, log2BlockSize uint64) bool {
	if t.write == nil {
		return true
	}

	if ds.isWritingFullLine(t.write, log2BlockSize) {
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

	var (
		addr uint64
		pid  vm.PID
		req  sim.Msg
	)

	if trans.read != nil {
		req = trans.read
		addr = trans.read.Address
		pid = trans.read.PID
	} else {
		req = trans.write
		addr = trans.write.Address
		pid = trans.write.PID
	}

	cacheLineID, _ := getCacheLineID(addr, spec.Log2BlockSize)

	bankNum := bankID(setID, wayID,
		spec.WayAssociativity, len(ds.cache.dirToBankBuffers))
	bankBuf := ds.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	mshrIdx := cache.MSHRAdd(
		&next.MSHRState, spec.NumMSHREntry,
		pid, cacheLineID)
	trans.mshrEntryIndex = mshrIdx
	trans.hasMSHREntry = true

	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true

	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	block.IsLocked = true
	block.Tag = cacheLineID
	block.PID = uint32(pid)
	block.IsValid = true
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(req, ds.cache),
		ds.cache,
		fmt.Sprintf("add-mshr-entry-0x%x-0x%x",
			next.MSHRState.Entries[mshrIdx].Address,
			block.Tag),
	)

	ds.popDirPostBuf()

	trans.action = writeBufferFetch
	trans.fetchPID = pid
	trans.fetchAddress = cacheLineID
	bankBuf.Push(trans)

	entry := &next.MSHRState.Entries[mshrIdx]
	entry.BlockSetID = setID
	entry.BlockWayID = wayID
	entry.HasBlock = true
	entry.TransactionIndices = append(
		entry.TransactionIndices, ds.findTransIndex(trans))

	return true
}

func (ds *directoryStage) isWritingFullLine(write *mem.WriteReq, log2BlockSize uint64) bool {
	if len(write.Data) != (1 << log2BlockSize) {
		return false
	}

	if write.DirtyMask != nil {
		for _, dirty := range write.DirtyMask {
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
	if len(next.DirPostPipelineBufIndices) > 0 {
		next.DirPostPipelineBufIndices = next.DirPostPipelineBufIndices[1:]
	}
}
