package writethrough

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type dirPipelineItem struct {
	trans *transaction
}

func (i dirPipelineItem) TaskID() string {
	return i.trans.id + "_dir_pipeline"
}

type directory struct {
	cache    *Comp
	pipeline pipelining.Pipeline
	buf      sim.Buffer
}

func (d *directory) Tick() (madeProgress bool) {
	for i := 0; i < d.cache.numReqPerCycle; i++ {
		if !d.pipeline.CanAccept() {
			break
		}

		item := d.cache.dirBuf.Peek()
		if item == nil {
			break
		}

		trans := item.(*transaction)
		d.pipeline.Accept(dirPipelineItem{trans})
		d.cache.dirBuf.Pop()

		madeProgress = true
	}

	madeProgress = d.pipeline.Tick() || madeProgress

	for i := 0; i < d.cache.numReqPerCycle; i++ {
		item := d.buf.Peek()
		if item == nil {
			break
		}

		trans := item.(dirPipelineItem).trans

		if trans.read != nil {
			madeProgress = d.processRead(trans) || madeProgress
			continue
		}

		madeProgress = d.processWrite(trans) || madeProgress
	}

	return madeProgress
}

func (d *directory) processRead(trans *transaction) bool {
	read := trans.read
	addr := read.Address
	pid := read.PID
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.cache.mshr.Query(pid, cacheLineID)
	if mshrEntry != nil {
		return d.processMSHRHit(trans, mshrEntry)
	}

	block := d.cache.directory.Lookup(pid, cacheLineID)
	if block != nil && block.IsValid {
		return d.processReadHit(trans, block)
	}

	return d.processReadMiss(trans)
}

func (d *directory) processMSHRHit(
	trans *transaction,
	mshrEntry *cache.MSHREntry,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, trans)

	d.buf.Pop()

	if trans.read != nil {
		tracing.AddTaskStep(trans.id, d.cache, "read-mshr-hit")
	} else {
		tracing.AddTaskStep(trans.id, d.cache, "write-mshr-hit")
	}

	return true
}

func (d *directory) processReadHit(
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked {
		return false
	}

	bankBuf := d.getBankBuf(block)
	if !bankBuf.CanPush() {
		return false
	}

	trans.block = block
	trans.bankAction = bankActionReadHit
	block.ReadCount++
	d.cache.directory.Visit(block)
	bankBuf.Push(trans)

	d.buf.Pop()
	tracing.AddTaskStep(trans.id, d.cache, "read-hit")

	return true
}

func (d *directory) processReadMiss(
	trans *transaction,
) bool {
	read := trans.read
	addr := read.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	victim := d.cache.directory.FindVictim(cacheLineID)
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if d.cache.mshr.IsFull() {
		return false
	}

	if !d.fetchFromBottom(trans, victim) {
		return false
	}

	d.buf.Pop()
	tracing.AddTaskStep(trans.id, d.cache, "read-miss")

	return true
}

func (d *directory) processWrite(
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	pid := write.PID
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.cache.mshr.Query(pid, cacheLineID)
	if mshrEntry != nil {
		ok := d.writeBottom(trans)
		if ok {
			return d.processMSHRHit(trans, mshrEntry)
		}

		return false
	}

	block := d.cache.directory.Lookup(pid, cacheLineID)
	if block != nil && block.IsValid {
		ok := d.processWriteHit(trans, block)
		if ok {
			tracing.AddTaskStep(trans.id, d.cache, "write-hit")
		}

		return ok
	}

	if d.isPartialWrite(write) {
		return d.partialWriteMiss(trans)
	}

	ok := d.fullLineWriteMiss(trans)
	if ok {
		tracing.AddTaskStep(trans.id, d.cache, "write-miss")
	}

	return ok
}

func (d *directory) isPartialWrite(write *mem.WriteReq) bool {
	if len(write.Data) < (1 << d.cache.log2BlockSize) {
		return true
	}

	if write.DirtyMask != nil {
		for _, byteDirty := range write.DirtyMask {
			if !byteDirty {
				return true
			}
		}
	}

	return false
}

func (d *directory) partialWriteMiss(
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	trans.fetchAndWrite = true

	if d.cache.mshr.IsFull() {
		return false
	}

	victim := d.cache.directory.FindVictim(cacheLineID)
	if victim.ReadCount > 0 || victim.IsLocked {
		return false
	}

	sentThisCycle := false

	if trans.writeToBottom == nil {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}

		sentThisCycle = true
	}

	ok := d.fetchFromBottom(trans, victim)
	if !ok {
		return sentThisCycle
	}

	d.buf.Pop()
	tracing.AddTaskStep(trans.id, d.cache, "write-miss")

	return true
}

func (d *directory) fullLineWriteMiss(
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block := d.cache.directory.FindVictim(cacheLineID)

	return d.processWriteHit(trans, block)
}

func (d *directory) writeBottom(trans *transaction) bool {
	write := trans.write
	addr := write.Address

	writeToBottom := mem.WriteReqBuilder{}.
		WithSrc(d.cache.bottomPort.AsRemote()).
		WithDst(d.cache.addressToPortMapper.Find(addr)).
		WithAddress(addr).
		WithPID(write.PID).
		WithData(write.Data).
		WithDirtyMask(write.DirtyMask).
		Build()

	err := d.cache.bottomPort.Send(writeToBottom)
	if err != nil {
		return false
	}

	trans.writeToBottom = writeToBottom

	tracing.TraceReqInitiate(writeToBottom, d.cache, trans.id)

	return true
}

func (d *directory) processWriteHit(
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(block)
	if !bankBuf.CanPush() {
		return false
	}

	if trans.writeToBottom == nil {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block.IsLocked = true
	block.IsValid = true
	block.Tag = cacheLineID
	d.cache.directory.Visit(block)

	trans.bankAction = bankActionWrite
	trans.block = block
	bankBuf.Push(trans)

	d.buf.Pop()

	return true
}

func (d *directory) fetchFromBottom(
	trans *transaction,
	victim *cache.Block,
) bool {
	addr := trans.Address()
	pid := trans.PID()
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	bottomModule := d.cache.addressToPortMapper.Find(cacheLineID)
	readToBottom := mem.ReadReqBuilder{}.
		WithSrc(d.cache.bottomPort.AsRemote()).
		WithDst(bottomModule).
		WithAddress(cacheLineID).
		WithPID(pid).
		WithByteSize(blockSize).
		Build()
	err := d.cache.bottomPort.Send(readToBottom)

	if err != nil {
		return false
	}

	tracing.TraceReqInitiate(readToBottom, d.cache, trans.id)
	trans.readToBottom = readToBottom
	trans.block = victim

	mshrEntry := d.cache.mshr.Add(pid, cacheLineID)
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	mshrEntry.ReadReq = readToBottom
	mshrEntry.Block = victim

	victim.Tag = cacheLineID
	victim.PID = pid
	victim.IsValid = true
	victim.IsLocked = true
	d.cache.directory.Visit(victim)

	return true
}

func (d *directory) getBankBuf(block *cache.Block) sim.Buffer {
	numWaysPerSet := d.cache.directory.WayAssociativity()
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(d.cache.bankBufs)

	return d.cache.bankBufs[bankID]
}
