package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type dirPipelineItem struct {
	trans *transactionState
}

func (i dirPipelineItem) TaskID() string {
	return i.trans.id + "_dir_pipeline"
}

type directory struct {
	cache *pipelineMW
}

func (d *directory) Tick() (madeProgress bool) {
	spec := d.cache.GetSpec()
	cur := d.cache.comp.GetState()
	next := d.cache.comp.GetNextState()

	// Accept from dirBuf into pipeline
	for i := 0; i < spec.NumReqPerCycle; i++ {
		if !dirPipelineCanAccept(
			cur.DirPipelineStages, spec.NumReqPerCycle) {
			break
		}

		item := d.cache.dirBufAdapter.Peek()
		if item == nil {
			break
		}

		trans := item.(*transactionState)
		transIdx := d.findPostCoalesceTransIdx(trans)
		dirPipelineAccept(
			&next.DirPipelineStages, spec.NumReqPerCycle, transIdx)
		d.cache.dirBufAdapter.Pop()

		madeProgress = true
	}

	// Tick pipeline
	madeProgress = dirPipelineTick(
		&next.DirPipelineStages,
		&next.DirPostPipelineBufIndices,
		spec.NumReqPerCycle,
		spec.DirLatency,
	) || madeProgress

	// Process items from post-pipeline buffer
	for i := 0; i < spec.NumReqPerCycle; i++ {
		item := d.cache.dirPostBufAdapter.Peek()
		if item == nil {
			break
		}

		trans := item.(dirPipelineItem).trans

		var processed bool
		if trans.read != nil {
			processed = d.processRead(trans)
		} else {
			processed = d.processWrite(trans)
		}

		if !processed {
			break
		}

		madeProgress = true
	}

	return madeProgress
}

func (d *directory) processRead(trans *transactionState) bool {
	addr := trans.read.Address
	pid := trans.read.PID
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	cur := d.cache.comp.GetState()

	entryIdx, mshrFound := cache.MSHRQuery(
		&cur.MSHRState, pid, cacheLineID)
	if mshrFound {
		return d.processMSHRHit(trans, entryIdx)
	}

	setID, wayID, found := cache.DirectoryLookup(
		&cur.DirectoryState, spec.NumSets, int(blockSize),
		pid, cacheLineID)
	if found && cur.DirectoryState.Sets[setID].Blocks[wayID].IsValid {
		return d.processReadHit(trans, setID, wayID)
	}

	return d.processReadMiss(trans)
}

func (d *directory) processMSHRHit(
	trans *transactionState,
	entryIdx int,
) bool {
	next := d.cache.comp.GetNextState()

	next.MSHRState.Entries[entryIdx].TransactionIndices =
		append(next.MSHRState.Entries[entryIdx].TransactionIndices,
			d.findPostCoalesceTransIdx(trans))

	if trans.read != nil {
		tracing.AddTaskStep(trans.id, d.cache.comp, "read-mshr-hit")
	} else {
		tracing.AddTaskStep(trans.id, d.cache.comp, "write-mshr-hit")
	}

	d.cache.dirPostBufAdapter.Pop()

	return true
}

func (d *directory) processReadHit(
	trans *transactionState,
	setID, wayID int,
) bool {
	cur := d.cache.comp.GetState()
	next := d.cache.comp.GetNextState()
	block := &cur.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true
	trans.bankAction = bankActionReadHit

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.ReadCount++
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)
	bankBuf.Push(trans)

	d.cache.dirPostBufAdapter.Pop()
	tracing.AddTaskStep(trans.id, d.cache.comp, "read-hit")

	return true
}

func (d *directory) processReadMiss(trans *transactionState) bool {
	addr := trans.read.Address
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	cur := d.cache.comp.GetState()
	next := d.cache.comp.GetNextState()

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&cur.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)
	victim := &cur.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if cache.MSHRIsFull(&cur.MSHRState, spec.NumMSHREntry) {
		return false
	}

	if !d.fetchFromBottom(trans, victimSetID, victimWayID) {
		return false
	}

	_ = next // writes done in fetchFromBottom

	d.cache.dirPostBufAdapter.Pop()
	tracing.AddTaskStep(trans.id, d.cache.comp, "read-miss")

	return true
}

func (d *directory) processWrite(trans *transactionState) bool {
	addr := trans.write.Address
	pid := trans.write.PID
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	cur := d.cache.comp.GetState()

	entryIdx, mshrFound := cache.MSHRQuery(
		&cur.MSHRState, pid, cacheLineID)
	if mshrFound {
		ok := d.writeBottom(trans)
		if ok {
			return d.processMSHRHit(trans, entryIdx)
		}

		return false
	}

	setID, wayID, found := cache.DirectoryLookup(
		&cur.DirectoryState, spec.NumSets, int(blockSize),
		pid, cacheLineID)
	if found && cur.DirectoryState.Sets[setID].Blocks[wayID].IsValid {
		return d.processWriteHit(trans, setID, wayID)
	}

	return d.writeMiss(trans)
}

func (d *directory) writeMiss(trans *transactionState) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskStep(trans.id, d.cache.comp, "write-miss")
		d.cache.dirPostBufAdapter.Pop()

		return true
	}

	return false
}

func (d *directory) writeBottom(trans *transactionState) bool {
	addr := trans.write.Address

	writeToBottom := &mem.WriteReq{}
	writeToBottom.ID = sim.GetIDGenerator().Generate()
	writeToBottom.Src = d.cache.bottomPort.AsRemote()
	writeToBottom.Dst = d.cache.findPort(addr)
	writeToBottom.Address = addr
	writeToBottom.PID = trans.write.PID
	writeToBottom.Data = trans.write.Data
	writeToBottom.DirtyMask = trans.write.DirtyMask
	writeToBottom.TrafficBytes = len(trans.write.Data) + 12
	writeToBottom.TrafficClass = "req"

	err := d.cache.bottomPort.Send(writeToBottom)
	if err != nil {
		return false
	}

	trans.writeToBottom = writeToBottom

	tracing.TraceReqInitiate(writeToBottom, d.cache.comp, trans.id)

	return true
}

func (d *directory) processWriteHit(
	trans *transactionState,
	setID, wayID int,
) bool {
	cur := d.cache.comp.GetState()
	next := d.cache.comp.GetNextState()
	block := &cur.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	if trans.writeToBottom == nil {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	addr := trans.write.Address
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsLocked = true
	nextBlock.IsValid = true
	nextBlock.Tag = cacheLineID
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	trans.bankAction = bankActionWrite
	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true
	bankBuf.Push(trans)

	tracing.AddTaskStep(trans.id, d.cache.comp, "write-hit")
	d.cache.dirPostBufAdapter.Pop()

	return true
}

func (d *directory) fetchFromBottom(
	trans *transactionState,
	victimSetID, victimWayID int,
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

	tracing.TraceReqInitiate(readToBottom, d.cache.comp, trans.id)
	trans.readToBottom = readToBottom
	trans.blockSetID = victimSetID
	trans.blockWayID = victimWayID
	trans.hasBlock = true

	entryIdx := cache.MSHRAdd(
		&next.MSHRState, spec.NumMSHREntry, pid, cacheLineID)
	entry := &next.MSHRState.Entries[entryIdx]
	entry.TransactionIndices = append(entry.TransactionIndices,
		d.findPostCoalesceTransIdx(trans))
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

func (d *directory) getBankBuf(setID, wayID int) *stateTransBuffer {
	numWaysPerSet := d.cache.GetSpec().WayAssociativity
	blockID := setID*numWaysPerSet + wayID
	bankID := blockID % len(d.cache.bankBufAdapters)

	return d.cache.bankBufAdapters[bankID]
}

// findPostCoalesceTransIdx returns the index of trans in
// postCoalesceTransactions.
func (d *directory) findPostCoalesceTransIdx(trans *transactionState) int {
	for i, t := range d.cache.postCoalesceTransactions {
		if t != nil && t == trans {
			return i
		}
	}

	panic("transaction not found in postCoalesceTransactions")
}
