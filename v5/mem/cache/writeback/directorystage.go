package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/queueing"
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
	cache    *middleware
	pipeline queueing.Pipeline
	buf      queueing.Buffer
}

func (ds *directoryStage) Tick() (madeProgress bool) {
	madeProgress = ds.acceptNewTransaction() || madeProgress

	madeProgress = ds.pipeline.Tick() || madeProgress

	madeProgress = ds.processTransaction() || madeProgress

	return madeProgress
}

func (ds *directoryStage) processTransaction() bool {
	madeProgress := false

	for i := 0; i < ds.cache.numReqPerCycle; i++ {
		item := ds.buf.Peek()
		if item == nil {
			break
		}

		trans := item.(dirPipelineItem).trans

		addr := trans.accessReq().GetAddress()
		cacheLineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

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

	for i := 0; i < ds.cache.numReqPerCycle; i++ {
		if !ds.pipeline.CanAccept() {
			break
		}

		item := ds.cache.dirStageBuffer.Peek()
		if item == nil {
			break
		}

		trans := item.(*transactionState)
		ds.pipeline.Accept(dirPipelineItem{trans})
		ds.cache.dirStageBuffer.Pop()

		madeProgress = true
	}

	return madeProgress
}

func (ds *directoryStage) Reset() {
	ds.pipeline.Clear()
	ds.buf.Clear()
	ds.cache.dirStageBuffer.Clear()
}

func (ds *directoryStage) doRead(trans *transactionState) bool {
	read := trans.read
	cachelineID, _ := getCacheLineID(read.Address, ds.cache.log2BlockSize)

	mshrIdx, found := cache.MSHRQuery(
		&ds.cache.mshrState, read.PID, cachelineID)
	if found {
		return ds.handleReadMSHRHit(trans, mshrIdx)
	}

	setID, wayID, blockFound := cache.DirectoryLookup(
		&ds.cache.directoryState,
		ds.cache.numSets, ds.cache.blockSize,
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
	trans.mshrEntryIndex = mshrIdx
	trans.hasMSHREntry = true
	ds.cache.mshrState.Entries[mshrIdx].TransactionIndices = append(
		ds.cache.mshrState.Entries[mshrIdx].TransactionIndices,
		ds.findTransIndex(trans))

	ds.buf.Pop()

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
	block := &ds.cache.directoryState.Sets[setID].Blocks[wayID]
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
	cacheLineID, _ := getCacheLineID(read.Address, ds.cache.log2BlockSize)

	if cache.MSHRIsFull(&ds.cache.mshrState, ds.cache.numMSHREntry) {
		return false
	}

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&ds.cache.directoryState,
		ds.cache.numSets, ds.cache.blockSize,
		cacheLineID)
	victim := &ds.cache.directoryState.Sets[victimSetID].Blocks[victimWayID]

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
	cachelineID, _ := getCacheLineID(write.Address, ds.cache.log2BlockSize)

	mshrIdx, found := cache.MSHRQuery(
		&ds.cache.mshrState, write.PID, cachelineID)
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
		&ds.cache.directoryState,
		ds.cache.numSets, ds.cache.blockSize,
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
	trans.mshrEntryIndex = mshrIdx
	trans.hasMSHREntry = true
	ds.cache.mshrState.Entries[mshrIdx].TransactionIndices = append(
		ds.cache.mshrState.Entries[mshrIdx].TransactionIndices,
		ds.findTransIndex(trans))

	ds.buf.Pop()

	return true
}

func (ds *directoryStage) doWriteHit(
	trans *transactionState,
	setID, wayID int,
) bool {
	block := &ds.cache.directoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	return ds.writeToBank(trans, setID, wayID)
}

func (ds *directoryStage) doWriteMiss(trans *transactionState) bool {
	if ds.isWritingFullLine(trans.write) {
		return ds.writeFullLineMiss(trans)
	}

	return ds.writePartialLineMiss(trans)
}

func (ds *directoryStage) writeFullLineMiss(trans *transactionState) bool {
	write := trans.write
	cachelineID, _ := getCacheLineID(write.Address, ds.cache.log2BlockSize)

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&ds.cache.directoryState,
		ds.cache.numSets, ds.cache.blockSize,
		cachelineID)
	victim := &ds.cache.directoryState.Sets[victimSetID].Blocks[victimWayID]

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
	cachelineID, _ := getCacheLineID(write.Address, ds.cache.log2BlockSize)

	if cache.MSHRIsFull(&ds.cache.mshrState, ds.cache.numMSHREntry) {
		return false
	}

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&ds.cache.directoryState,
		ds.cache.numSets, ds.cache.blockSize,
		cachelineID)
	victim := &ds.cache.directoryState.Sets[victimSetID].Blocks[victimWayID]

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
	numBanks := len(ds.cache.dirToBankBuffers)
	bank := bankID(setID, wayID, ds.cache.wayAssociativity, numBanks)
	bankBuf := ds.cache.dirToBankBuffers[bank]

	if !bankBuf.CanPush() {
		return false
	}

	cache.DirectoryVisit(&ds.cache.directoryState, setID, wayID)

	block := &ds.cache.directoryState.Sets[setID].Blocks[wayID]
	block.ReadCount++
	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true
	trans.action = bankReadHit

	ds.buf.Pop()
	bankBuf.Push(trans)

	return true
}

func (ds *directoryStage) writeToBank(
	trans *transactionState,
	setID, wayID int,
) bool {
	numBanks := len(ds.cache.dirToBankBuffers)
	bank := bankID(setID, wayID, ds.cache.wayAssociativity, numBanks)
	bankBuf := ds.cache.dirToBankBuffers[bank]

	if !bankBuf.CanPush() {
		return false
	}

	write := trans.write
	addr := write.Address
	cachelineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

	cache.DirectoryVisit(&ds.cache.directoryState, setID, wayID)
	block := &ds.cache.directoryState.Sets[setID].Blocks[wayID]
	block.IsLocked = true
	block.Tag = cachelineID
	block.IsValid = true
	block.PID = uint32(write.PID)
	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true
	trans.action = bankWriteHit

	ds.buf.Pop()
	bankBuf.Push(trans)

	return true
}

func (ds *directoryStage) evict(
	trans *transactionState,
	victimSetID, victimWayID int,
) bool {
	bankNum := bankID(victimSetID, victimWayID,
		ds.cache.wayAssociativity, len(ds.cache.dirToBankBuffers))
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

	cacheLineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

	ds.updateTransForEviction(trans, victimSetID, victimWayID, pid, cacheLineID)
	ds.updateVictimBlockMetaData(victimSetID, victimWayID, cacheLineID, pid)

	ds.buf.Pop()
	bankBuf.Push(trans)

	ds.cache.evictingList[trans.victimTag] = true

	return true
}

func (ds *directoryStage) updateVictimBlockMetaData(
	setID, wayID int,
	cacheLineID uint64,
	pid vm.PID,
) {
	block := &ds.cache.directoryState.Sets[setID].Blocks[wayID]
	block.Tag = cacheLineID
	block.PID = uint32(pid)
	block.IsLocked = true
	block.IsDirty = false
	cache.DirectoryVisit(&ds.cache.directoryState, setID, wayID)
}

func (ds *directoryStage) updateTransForEviction(
	trans *transactionState,
	victimSetID, victimWayID int,
	pid vm.PID,
	cacheLineID uint64,
) {
	victim := &ds.cache.directoryState.Sets[victimSetID].Blocks[victimWayID]

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

	if ds.evictionNeedFetch(trans) {
		mshrIdx := cache.MSHRAdd(
			&ds.cache.mshrState, ds.cache.numMSHREntry,
			pid, cacheLineID)
		entry := &ds.cache.mshrState.Entries[mshrIdx]
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

func (ds *directoryStage) evictionNeedFetch(t *transactionState) bool {
	if t.write == nil {
		return true
	}

	if ds.isWritingFullLine(t.write) {
		return false
	}

	return true
}

func (ds *directoryStage) fetch(
	trans *transactionState,
	setID, wayID int,
) bool {
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

	cacheLineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

	bankNum := bankID(setID, wayID,
		ds.cache.wayAssociativity, len(ds.cache.dirToBankBuffers))
	bankBuf := ds.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	mshrIdx := cache.MSHRAdd(
		&ds.cache.mshrState, ds.cache.numMSHREntry,
		pid, cacheLineID)
	trans.mshrEntryIndex = mshrIdx
	trans.hasMSHREntry = true

	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true

	block := &ds.cache.directoryState.Sets[setID].Blocks[wayID]
	block.IsLocked = true
	block.Tag = cacheLineID
	block.PID = uint32(pid)
	block.IsValid = true
	cache.DirectoryVisit(&ds.cache.directoryState, setID, wayID)

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(req, ds.cache),
		ds.cache,
		fmt.Sprintf("add-mshr-entry-0x%x-0x%x",
			ds.cache.mshrState.Entries[mshrIdx].Address,
			block.Tag),
	)

	ds.buf.Pop()

	trans.action = writeBufferFetch
	trans.fetchPID = pid
	trans.fetchAddress = cacheLineID
	bankBuf.Push(trans)

	entry := &ds.cache.mshrState.Entries[mshrIdx]
	entry.BlockSetID = setID
	entry.BlockWayID = wayID
	entry.HasBlock = true
	entry.TransactionIndices = append(
		entry.TransactionIndices, ds.findTransIndex(trans))

	return true
}

func (ds *directoryStage) isWritingFullLine(write *mem.WriteReq) bool {
	if len(write.Data) != (1 << ds.cache.log2BlockSize) {
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
