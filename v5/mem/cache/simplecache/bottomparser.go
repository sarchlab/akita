package simplecache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	"github.com/sarchlab/akita/v5/tracing"
)

type bottomParser struct {
	cache *pipelineMW
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

	if p.cache.writePolicy.NeedsDualCompletion() {
		trans.bottomWriteDone = true

		if trans.bankDone {
			p.removeTransaction(trans)
			tracing.EndTask(trans.id, p.cache.comp)
		}
	} else {
		p.removeTransaction(trans)
		tracing.EndTask(trans.id, p.cache.comp)
	}

	p.cache.bottomPort.RetrieveIncoming()

	tracing.TraceReqFinalize(trans.writeToBottom, p.cache.comp)

	return true
}

func (p *bottomParser) processDataReady(msg sim.Msg) bool {
	trans := p.findTransactionByReadToBottomID(msg.Meta().RspTo)
	if trans == nil {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	bankBuf := p.getBankBuf(trans.blockSetID, trans.blockWayID)
	if !bankBuf.CanPush() {
		return false
	}

	pid := trans.readToBottom.PID
	addr := trans.Address()
	spec := p.cache.GetSpec()
	cachelineID := (addr >> spec.Log2BlockSize) << spec.Log2BlockSize
	drMsg := msg.(*mem.DataReadyRsp)
	data := drMsg.Data
	dirtyMask := make([]bool, 1<<spec.Log2BlockSize)
	next := p.cache.comp.GetNextState()

	entryIdx, found := cache.MSHRQuery(&next.MSHRState, pid, cachelineID)
	if !found {
		panic("MSHR entry not found for data ready response")
	}

	entry := &next.MSHRState.Entries[entryIdx]
	blockTag := next.DirectoryState.Sets[entry.BlockSetID].Blocks[entry.BlockWayID].Tag

	// Resolve transaction pointers before any removals shift indices
	entryTrans := p.resolveEntryTransactions(entry)
	p.mergeMSHRData(entryTrans, blockTag, data, dirtyMask)

	// Set up trans for bank processing and push BEFORE any removals
	trans.bankAction = bankActionWriteFetched
	trans.data = data
	trans.writeFetchedDirtyMask = dirtyMask

	transIdx := p.findPostCoalesceIdx(trans)
	bankBuf.PushTyped(transIdx)

	// Finalize MSHR transactions (marks pre-coalesce as done, removes
	// from postCoalesceTransactions). Skip the fetcher trans — it's been
	// pushed to the bank buffer and will be removed after bank processing.
	p.finalizeMSHRTransExcept(entryTrans, blockTag, data, trans)
	cache.MSHRRemove(&next.MSHRState, pid, cachelineID)

	p.cache.bottomPort.RetrieveIncoming()

	tracing.TraceReqFinalize(trans.readToBottom, p.cache.comp)

	return true
}

// resolveEntryTransactions collects the actual transaction pointers from
// the MSHR entry's TransactionIndices. This must be done before any
// removeTransaction calls, since those shift the slice indices.
func (p *bottomParser) resolveEntryTransactions(
	entry *cache.MSHREntryState,
) []*transactionState {
	result := make([]*transactionState, len(entry.TransactionIndices))
	for i, transIdx := range entry.TransactionIndices {
		result[i] = p.cache.postCoalesceTransactions[transIdx]
	}
	return result
}

func (p *bottomParser) mergeMSHRData(
	entryTrans []*transactionState,
	blockTag uint64,
	data []byte,
	dirtyMask []bool,
) {
	for _, trans := range entryTrans {
		if trans.write == nil {
			continue
		}

		offset := trans.write.Address - blockTag

		for i := 0; i < len(trans.write.Data); i++ {
			if trans.write.DirtyMask[i] {
				data[offset+uint64(i)] = trans.write.Data[i]
				dirtyMask[offset+uint64(i)] = true
			}
		}
	}
}

func (p *bottomParser) finalizeMSHRTransExcept(
	entryTrans []*transactionState,
	blockTag uint64,
	data []byte,
	except *transactionState,
) {
	for _, trans := range entryTrans {
		if trans.read != nil {
			for _, preCTrans := range trans.preCoalesceTransactions {
				offset := preCTrans.read.Address - blockTag
				preCTrans.data = data[offset : offset+preCTrans.read.AccessByteSize]
				preCTrans.done = true
			}
		} else {
			for _, preCTrans := range trans.preCoalesceTransactions {
				preCTrans.done = true
			}
		}

		if trans != except {
			p.removeTransaction(trans)
		}

		tracing.EndTask(trans.id, p.cache.comp)
	}
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id string,
) *transactionState {
	for _, trans := range p.cache.postCoalesceTransactions {
		if trans != nil && trans.writeToBottom != nil && trans.writeToBottom.ID == id {
			return trans
		}
	}

	return nil
}

func (p *bottomParser) findTransactionByReadToBottomID(
	id string,
) *transactionState {
	for _, trans := range p.cache.postCoalesceTransactions {
		if trans != nil && trans.readToBottom != nil && trans.readToBottom.ID == id {
			return trans
		}
	}

	return nil
}

func (p *bottomParser) removeTransaction(trans *transactionState) {
	for i, t := range p.cache.postCoalesceTransactions {
		if t == trans {
			p.cache.postCoalesceTransactions[i] = nil

			return
		}
	}
}

func (p *bottomParser) findPostCoalesceIdx(
	trans *transactionState,
) int {
	for i, t := range p.cache.postCoalesceTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in postCoalesceTransactions")
}

func (p *bottomParser) getBankBuf(
	setID, wayID int,
) *stateutil.Buffer[int] {
	next := p.cache.comp.GetNextState()
	numWaysPerSet := p.cache.GetSpec().WayAssociativity
	blockID := setID*numWaysPerSet + wayID
	bankID := blockID % len(next.BankBufs)

	return &next.BankBufs[bankID]
}
