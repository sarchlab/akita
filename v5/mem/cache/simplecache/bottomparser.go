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
	next := p.cache.comp.GetNextState()
	transIdx := p.findTransactionByWriteToBottomID(msg.Meta().RspTo)
	if transIdx < 0 {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	trans := next.postCoalesceTrans(transIdx)
	if trans.FetchAndWrite {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	for _, preIdx := range trans.PreCoalesceTransIdxs {
		next.Transactions[preIdx].Done = true
	}

	if p.cache.writePolicy.NeedsDualCompletion() {
		trans.BottomWriteDone = true

		if trans.BankDone {
			p.removeTransaction(trans)
			tracing.EndTask(trans.ID, p.cache.comp)
		}
	} else {
		p.removeTransaction(trans)
		tracing.EndTask(trans.ID, p.cache.comp)
	}

	p.cache.bottomPort.RetrieveIncoming()

	// Reconstruct writeToBottom for tracing
	writeToBottom := &mem.WriteReq{
		MsgMeta:   trans.WriteToBottomMeta,
		Data:      trans.WriteToBottomData,
		DirtyMask: trans.WriteToBottomDirtyMask,
		PID:       trans.WriteToBottomPID,
	}
	tracing.TraceReqFinalize(writeToBottom, p.cache.comp)

	return true
}

func (p *bottomParser) processDataReady(msg sim.Msg) bool {
	next := p.cache.comp.GetNextState()
	transIdx := p.findTransactionByReadToBottomID(msg.Meta().RspTo)
	if transIdx < 0 {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	trans := next.postCoalesceTrans(transIdx)

	bankBuf := p.getBankBuf(trans.BlockSetID, trans.BlockWayID)
	if !bankBuf.CanPush() {
		return false
	}

	pid := trans.ReadToBottomPID
	addr := trans.Address()
	spec := p.cache.GetSpec()
	cachelineID := (addr >> spec.Log2BlockSize) << spec.Log2BlockSize
	drMsg := msg.(*mem.DataReadyRsp)
	data := drMsg.Data
	dirtyMask := make([]bool, 1<<spec.Log2BlockSize)

	entryIdx, found := cache.MSHRQuery(&next.MSHRState, pid, cachelineID)
	if !found {
		panic("MSHR entry not found for data ready response")
	}

	entry := &next.MSHRState.Entries[entryIdx]
	blockTag := next.DirectoryState.Sets[entry.BlockSetID].Blocks[entry.BlockWayID].Tag

	// Resolve entry transactions before any removals
	entryTransIdxs := append([]int(nil), entry.TransactionIndices...)
	p.mergeMSHRData(next, entryTransIdxs, blockTag, data, dirtyMask)

	// Set up trans for bank processing and push BEFORE any removals
	trans.BankAction = bankActionWriteFetched
	trans.Data = data
	trans.WriteFetchedDirtyMask = dirtyMask

	bankBuf.PushTyped(transIdx)

	// Finalize MSHR transactions (marks pre-coalesce as done, removes
	// from postCoalesceTransactions). Skip the fetcher trans — it's been
	// pushed to the bank buffer and will be removed after bank processing.
	p.finalizeMSHRTransExcept(next, entryTransIdxs, blockTag, data, transIdx)
	cache.MSHRRemove(&next.MSHRState, pid, cachelineID)

	p.cache.bottomPort.RetrieveIncoming()

	// Reconstruct readToBottom for tracing
	readToBottom := &mem.ReadReq{
		MsgMeta: trans.ReadToBottomMeta,
		PID:     trans.ReadToBottomPID,
	}
	tracing.TraceReqFinalize(readToBottom, p.cache.comp)

	return true
}

func (p *bottomParser) mergeMSHRData(
	next *State,
	entryTransIdxs []int,
	blockTag uint64,
	data []byte,
	dirtyMask []bool,
) {
	for _, pcIdx := range entryTransIdxs {
		trans := next.postCoalesceTrans(pcIdx)
		if !trans.HasWrite {
			continue
		}

		offset := trans.WriteAddress - blockTag

		for i := 0; i < len(trans.WriteData); i++ {
			if trans.WriteDirtyMask[i] {
				data[offset+uint64(i)] = trans.WriteData[i]
				dirtyMask[offset+uint64(i)] = true
			}
		}
	}
}

func (p *bottomParser) finalizeMSHRTransExcept(
	next *State,
	entryTransIdxs []int,
	blockTag uint64,
	data []byte,
	exceptIdx int,
) {
	for _, pcIdx := range entryTransIdxs {
		trans := next.postCoalesceTrans(pcIdx)

		if trans.HasRead {
			for _, preIdx := range trans.PreCoalesceTransIdxs {
				preCTrans := &next.Transactions[preIdx]
				offset := preCTrans.ReadAddress - blockTag
				preCTrans.Data = data[offset : offset+preCTrans.ReadAccessByteSize]
				preCTrans.Done = true
			}
		} else {
			for _, preIdx := range trans.PreCoalesceTransIdxs {
				next.Transactions[preIdx].Done = true
			}
		}

		if pcIdx != exceptIdx {
			p.removeTransaction(trans)
		}

		tracing.EndTask(trans.ID, p.cache.comp)
	}
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id string,
) int {
	next := p.cache.comp.GetNextState()
	numPost := next.numPostCoalesce()

	for i := 0; i < numPost; i++ {
		trans := next.postCoalesceTrans(i)
		if !trans.Removed && trans.HasWriteToBottom &&
			trans.WriteToBottomMeta.ID == id {
			return i
		}
	}

	return -1
}

func (p *bottomParser) findTransactionByReadToBottomID(
	id string,
) int {
	next := p.cache.comp.GetNextState()
	numPost := next.numPostCoalesce()

	for i := 0; i < numPost; i++ {
		trans := next.postCoalesceTrans(i)
		if !trans.Removed && trans.HasReadToBottom &&
			trans.ReadToBottomMeta.ID == id {
			return i
		}
	}

	return -1
}

func (p *bottomParser) removeTransaction(trans *transactionState) {
	trans.Removed = true
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
