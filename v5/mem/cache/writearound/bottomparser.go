package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type bottomParser struct {
	cache *Comp
}

func (p *bottomParser) Tick() bool {
	itemI := p.cache.bottomPort.PeekIncoming()
	if itemI == nil {
		return false
	}

	switch itemI.(type) {
	case *mem.WriteDoneRsp:
		return p.processDoneRsp(itemI)
	case *mem.DataReadyRsp:
		return p.processDataReady(itemI)
	default:
		panic("cannot process response")
	}
}

func (p *bottomParser) processDoneRsp(msg sim.Msg) bool {
	trans := p.findTransactionByWriteToBottomID(msg.Meta().RspTo)
	if trans == nil || trans.fetchAndWrite {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	for _, t := range trans.preCoalesceTransactions {
		t.done = true
	}

	p.removeTransaction(trans)
	p.cache.bottomPort.RetrieveIncoming()

	tracing.TraceReqFinalize(trans.writeToBottom, p.cache)
	tracing.EndTask(trans.id, p.cache)

	return true
}

func (p *bottomParser) processDataReady(msg sim.Msg) bool {
	trans := p.findTransactionByReadToBottomID(msg.Meta().RspTo)
	if trans == nil {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	bankBuf := p.getBankBuf(trans.block)
	if !bankBuf.CanPush() {
		return false
	}

	pid := trans.readToBottom.PID
	addr := trans.Address()
	cachelineID := (addr >> p.cache.log2BlockSize) << p.cache.log2BlockSize
	drMsg := msg.(*mem.DataReadyRsp)
	data := drMsg.Data
	dirtyMask := make([]bool, 1<<p.cache.log2BlockSize)
	mshrEntry := p.cache.mshr.Query(pid, cachelineID)
	p.mergeMSHRData(mshrEntry, data, dirtyMask)
	p.finalizeMSHRTrans(mshrEntry, data)
	p.cache.mshr.Remove(pid, cachelineID)

	trans.bankAction = bankActionWriteFetched
	trans.data = data
	trans.writeFetchedDirtyMask = dirtyMask
	bankBuf.Push(trans)

	p.removeTransaction(trans)
	p.cache.bottomPort.RetrieveIncoming()

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

		offset := trans.write.Address - mshrEntry.Block.Tag

		for i := 0; i < len(trans.write.Data); i++ {
			if trans.write.DirtyMask[i] {
				data[offset+uint64(i)] = trans.write.Data[i]
				dirtyMask[offset+uint64(i)] = true
			}
		}
	}
}

func (p *bottomParser) finalizeMSHRTrans(
	mshrEntry *cache.MSHREntry,
	data []byte,
) {
	for _, t := range mshrEntry.Requests {
		trans := t.(*transaction)
		if trans.read != nil {
			for _, preCTrans := range trans.preCoalesceTransactions {
				offset := preCTrans.read.Address - mshrEntry.Block.Tag
				preCTrans.data = data[offset : offset+preCTrans.read.AccessByteSize]
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

func (p *bottomParser) getBankBuf(block *cache.Block) queueing.Buffer {
	numWaysPerSet := p.cache.wayAssociativity
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(p.cache.bankBufs)

	return p.cache.bankBufs[bankID]
}
