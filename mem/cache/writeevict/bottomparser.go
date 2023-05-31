package writeevict

import (
	"github.com/sarchlab/akita/v3/mem/cache"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

type bottomParser struct {
	cache *Cache
}

func (p *bottomParser) Tick(now sim.VTimeInSec) bool {
	item := p.cache.bottomPort.Peek()
	if item == nil {
		return false
	}

	switch rsp := item.(type) {
	case *mem.WriteDoneRsp:
		return p.processDoneRsp(now, rsp)
	case *mem.DataReadyRsp:
		return p.processDataReady(now, rsp)
	default:
		panic("cannot process response")
	}
}

func (p *bottomParser) processDoneRsp(
	now sim.VTimeInSec,
	done *mem.WriteDoneRsp,
) bool {
	trans := p.findTransactionByWriteToBottomID(done.GetRspTo())
	if trans == nil || trans.fetchAndWrite {
		p.cache.bottomPort.Retrieve(now)
		return true
	}

	for _, t := range trans.preCoalesceTransactions {
		t.done = true
	}

	p.removeTransaction(trans)
	p.cache.bottomPort.Retrieve(now)

	tracing.TraceReqFinalize(trans.writeToBottom, p.cache)
	tracing.EndTask(trans.id, p.cache)

	return true
}

func (p *bottomParser) processDataReady(
	now sim.VTimeInSec,
	dr *mem.DataReadyRsp,
) bool {
	trans := p.findTransactionByReadToBottomID(dr.GetRspTo())
	if trans == nil {
		p.cache.bottomPort.Retrieve(now)
		return true
	}
	pid := trans.readToBottom.PID
	bankBuf := p.getBankBuf(trans.block)
	if !bankBuf.CanPush() {
		return false
	}

	addr := trans.Address()
	cachelineID := (addr >> p.cache.log2BlockSize) << p.cache.log2BlockSize
	data := dr.Data
	dirtyMask := make([]bool, 1<<p.cache.log2BlockSize)
	mshrEntry := p.cache.mshr.Query(pid, cachelineID)
	p.mergeMSHRData(mshrEntry, data, dirtyMask)
	p.finalizeMSHRTrans(mshrEntry, data, now)
	p.cache.mshr.Remove(pid, cachelineID)

	trans.bankAction = bankActionWriteFetched
	trans.data = data
	trans.writeFetchedDirtyMask = dirtyMask
	bankBuf.Push(trans)
	p.removeTransaction(trans)
	p.cache.bottomPort.Retrieve(now)

	tracing.TraceReqFinalize(trans.readToBottom, p.cache)

	return true
}

func (p *bottomParser) mergeMSHRData(
	mshrEntry *cache.MSHREntry,
	data []byte,
	dirtyMask []bool,
) {
	for _, t := range mshrEntry.Requests {
		trans := t.(*transaction)

		if trans.write == nil {
			continue
		}

		write := trans.write
		offset := write.Address - mshrEntry.Block.Tag
		for i := 0; i < len(write.Data); i++ {
			if write.DirtyMask[i] {
				data[offset+uint64(i)] = write.Data[i]
				dirtyMask[offset+uint64(i)] = true
			}
		}
	}
}

func (p *bottomParser) finalizeMSHRTrans(
	mshrEntry *cache.MSHREntry,
	data []byte,
	now sim.VTimeInSec,
) {
	for _, t := range mshrEntry.Requests {
		trans := t.(*transaction)
		if trans.read != nil {
			for _, preCTrans := range trans.preCoalesceTransactions {
				read := preCTrans.read
				offset := read.Address - mshrEntry.Block.Tag
				preCTrans.data = data[offset : offset+read.AccessByteSize]
				preCTrans.done = true
			}
		} else {
			for _, preCTrans := range trans.preCoalesceTransactions {
				preCTrans.done = true
			}
		}
		p.removeTransaction(trans)

		tracing.EndTask(trans.id, p.cache)
	}
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id string,
) *transaction {
	for _, trans := range p.cache.postCoalesceTransactions {
		if trans.writeToBottom != nil && trans.writeToBottom.ID == id {
			return trans
		}
	}
	return nil
}

func (p *bottomParser) findTransactionByReadToBottomID(
	id string,
) *transaction {
	for _, trans := range p.cache.postCoalesceTransactions {
		if trans.readToBottom != nil && trans.readToBottom.ID == id {
			return trans
		}
	}
	return nil
}

func (p *bottomParser) removeTransaction(trans *transaction) {
	for i, t := range p.cache.postCoalesceTransactions {
		if t == trans {
			p.cache.postCoalesceTransactions = append(
				(p.cache.postCoalesceTransactions)[:i],
				(p.cache.postCoalesceTransactions)[i+1:]...)
			return
		}
	}
}

func (p *bottomParser) getBankBuf(block *cache.Block) sim.Buffer {
	numWaysPerSet := p.cache.wayAssociativity
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(p.cache.bankBufs)
	return p.cache.bankBufs[bankID]
}
