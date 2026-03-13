package simplecache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	"github.com/sarchlab/akita/v5/tracing"
)

type directory struct {
	cache       *pipelineMW
	writePolicy WritePolicy
}

func (d *directory) Tick() (madeProgress bool) {
	spec := d.cache.GetSpec()
	next := d.cache.comp.GetNextState()
	dirBuf := &next.DirBuf
	dirPipeline := &next.DirPipeline
	dirPostBuf := &next.DirPostBuf

	// Accept from dirBuf into pipeline
	for i := 0; i < spec.NumReqPerCycle; i++ {
		if !dirPipeline.CanAccept() {
			break
		}

		item := dirBuf.Peek()
		if item == nil {
			break
		}

		transIdx := item.(int)
		dirPipeline.Accept(transIdx)
		dirBuf.Pop()

		madeProgress = true
	}

	// Tick pipeline
	madeProgress = dirPipeline.Tick(dirPostBuf) || madeProgress

	// Process items from post-pipeline buffer
	for i := 0; i < spec.NumReqPerCycle; i++ {
		item := dirPostBuf.Peek()
		if item == nil {
			break
		}

		transIdx := item.(int)
		trans := next.postCoalesceTrans(transIdx)

		var processed bool
		if trans.HasRead {
			processed = d.processRead(trans, transIdx)
		} else {
			processed = d.processWrite(trans, transIdx)
		}

		if !processed {
			break
		}

		madeProgress = true
	}

	return madeProgress
}

func (d *directory) processRead(trans *transactionState, postCoalesceIdx int) bool {
	addr := trans.ReadAddress
	pid := trans.ReadPID
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := d.cache.comp.GetNextState()

	entryIdx, mshrFound := cache.MSHRQuery(
		&next.MSHRState, pid, cacheLineID)
	if mshrFound {
		return d.processMSHRHit(trans, entryIdx, postCoalesceIdx)
	}

	setID, wayID, found := cache.DirectoryLookup(
		&next.DirectoryState, spec.NumSets, int(blockSize),
		pid, cacheLineID)
	if found && next.DirectoryState.Sets[setID].Blocks[wayID].IsValid {
		return d.processReadHit(trans, setID, wayID, postCoalesceIdx)
	}

	return d.processReadMiss(trans, postCoalesceIdx)
}

func (d *directory) processMSHRHit(
	trans *transactionState,
	entryIdx int,
	postCoalesceIdx int,
) bool {
	next := d.cache.comp.GetNextState()

	next.MSHRState.Entries[entryIdx].TransactionIndices =
		append(next.MSHRState.Entries[entryIdx].TransactionIndices,
			postCoalesceIdx)

	if trans.HasRead {
		tracing.AddTaskStep(trans.ID, d.cache.comp, "read-mshr-hit")
	} else {
		tracing.AddTaskStep(trans.ID, d.cache.comp, "write-mshr-hit")
	}

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

func (d *directory) processReadHit(
	trans *transactionState,
	setID, wayID int,
	postCoalesceIdx int,
) bool {
	next := d.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true
	trans.BankAction = bankActionReadHit

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.ReadCount++
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	bankBuf.PushTyped(postCoalesceIdx)

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()
	tracing.AddTaskStep(trans.ID, d.cache.comp, "read-hit")

	return true
}

func (d *directory) processReadMiss(trans *transactionState, postCoalesceIdx int) bool {
	addr := trans.ReadAddress
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := d.cache.comp.GetNextState()

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if cache.MSHRIsFull(&next.MSHRState, spec.NumMSHREntry) {
		return false
	}

	if !d.fetchFromBottom(trans, victimSetID, victimWayID, postCoalesceIdx) {
		return false
	}

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()
	tracing.AddTaskStep(trans.ID, d.cache.comp, "read-miss")

	return true
}

func (d *directory) processWrite(trans *transactionState, postCoalesceIdx int) bool {
	addr := trans.WriteAddress
	pid := trans.WritePID
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := d.cache.comp.GetNextState()

	entryIdx, mshrFound := cache.MSHRQuery(
		&next.MSHRState, pid, cacheLineID)
	if mshrFound {
		ok := d.writeBottom(trans)
		if ok {
			return d.processMSHRHit(trans, entryIdx, postCoalesceIdx)
		}

		return false
	}

	setID, wayID, found := cache.DirectoryLookup(
		&next.DirectoryState, spec.NumSets, int(blockSize),
		pid, cacheLineID)
	if found && next.DirectoryState.Sets[setID].Blocks[wayID].IsValid {
		return d.writePolicy.HandleWriteHit(d, trans, setID, wayID, postCoalesceIdx)
	}

	return d.writePolicy.HandleWriteMiss(d, trans, postCoalesceIdx)
}

func (d *directory) writeBottom(trans *transactionState) bool {
	addr := trans.WriteAddress

	writeToBottom := &mem.WriteReq{}
	writeToBottom.ID = sim.GetIDGenerator().Generate()
	writeToBottom.Src = d.cache.bottomPort.AsRemote()
	writeToBottom.Dst = d.cache.findPort(addr)
	writeToBottom.Address = addr
	writeToBottom.PID = trans.WritePID
	writeToBottom.Data = trans.WriteData
	writeToBottom.DirtyMask = trans.WriteDirtyMask
	writeToBottom.TrafficBytes = len(trans.WriteData) + 12
	writeToBottom.TrafficClass = "req"

	err := d.cache.bottomPort.Send(writeToBottom)
	if err != nil {
		return false
	}

	trans.HasWriteToBottom = true
	trans.WriteToBottomMeta = writeToBottom.MsgMeta
	trans.WriteToBottomPID = trans.WritePID
	trans.WriteToBottomData = trans.WriteData
	trans.WriteToBottomDirtyMask = trans.WriteDirtyMask

	tracing.TraceReqInitiate(writeToBottom, d.cache.comp, trans.ID)

	return true
}

func (d *directory) fetchFromBottom(
	trans *transactionState,
	victimSetID, victimWayID int,
	postCoalesceIdx int,
) bool {
	addr := trans.Address()
	pid := trans.PID()
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := d.cache.comp.GetNextState()

	bottomModule := d.cache.findPort(cacheLineID)
	readToBottom := &mem.ReadReq{}
	readToBottom.ID = sim.GetIDGenerator().Generate()
	readToBottom.Src = d.cache.bottomPort.AsRemote()
	readToBottom.Dst = bottomModule
	readToBottom.Address = cacheLineID
	readToBottom.PID = pid
	readToBottom.AccessByteSize = blockSize
	readToBottom.TrafficBytes = 12
	readToBottom.TrafficClass = "req"

	err := d.cache.bottomPort.Send(readToBottom)
	if err != nil {
		return false
	}

	tracing.TraceReqInitiate(readToBottom, d.cache.comp, trans.ID)

	trans.HasReadToBottom = true
	trans.ReadToBottomMeta = readToBottom.MsgMeta
	trans.ReadToBottomPID = pid
	trans.BlockSetID = victimSetID
	trans.BlockWayID = victimWayID
	trans.HasBlock = true

	entryIdx := cache.MSHRAdd(
		&next.MSHRState, spec.NumMSHREntry, pid, cacheLineID)
	entry := &next.MSHRState.Entries[entryIdx]
	entry.TransactionIndices = append(entry.TransactionIndices,
		postCoalesceIdx)
	entry.HasReadReq = true
	entry.ReadReq = readToBottom.MsgMeta
	entry.HasBlock = true
	entry.BlockSetID = victimSetID
	entry.BlockWayID = victimWayID

	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	victim.Tag = cacheLineID
	victim.PID = uint32(pid)
	victim.IsValid = true
	victim.IsLocked = true
	cache.DirectoryVisit(&next.DirectoryState, victimSetID, victimWayID)

	return true
}

func (d *directory) getBankBuf(setID, wayID int) *stateutil.Buffer[int] {
	next := d.cache.comp.GetNextState()
	numWaysPerSet := d.cache.GetSpec().WayAssociativity
	blockID := setID*numWaysPerSet + wayID
	bankID := blockID % len(next.BankBufs)

	return &next.BankBufs[bankID]
}
