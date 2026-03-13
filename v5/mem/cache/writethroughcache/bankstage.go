package writethroughcache

import (
	"github.com/sarchlab/akita/v5/tracing"
)

type bankStage struct {
	cache          *pipelineMW
	bankID         int
	numReqPerCycle int
}

func (s *bankStage) Reset() {
	next := s.cache.comp.GetNextState()
	next.BankPostBufs[s.bankID].Elements = nil
	next.BankPipelines[s.bankID].Stages = nil
}

func (s *bankStage) Tick() bool {
	madeProgress := false

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.tickPipeline() || madeProgress

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.extractFromBuf() || madeProgress
	}

	return madeProgress
}

func (s *bankStage) tickPipeline() bool {
	next := s.cache.comp.GetNextState()
	bankPipeline := &next.BankPipelines[s.bankID]
	bankPostBuf := &next.BankPostBufs[s.bankID]

	return bankPipeline.Tick(bankPostBuf)
}

func (s *bankStage) extractFromBuf() bool {
	next := s.cache.comp.GetNextState()
	bankBuf := &next.BankBufs[s.bankID]

	if bankBuf.Size() == 0 {
		return false
	}

	bankPipeline := &next.BankPipelines[s.bankID]
	if !bankPipeline.CanAccept() {
		return false
	}

	transIdx := bankBuf.Elements[0]
	bankPipeline.Accept(transIdx)
	bankBuf.Elements = bankBuf.Elements[1:]

	return true
}

func (s *bankStage) finalizeTrans() bool {
	next := s.cache.comp.GetNextState()
	bankPostBuf := &next.BankPostBufs[s.bankID]

	if bankPostBuf.Size() == 0 {
		return false
	}

	transIdx := bankPostBuf.Elements[0]
	trans := next.postCoalesceTrans(transIdx)

	switch trans.BankAction {
	case bankActionReadHit:
		return s.finalizeReadHitTrans(trans, transIdx)
	case bankActionWrite:
		return s.finalizeWriteTrans(trans, transIdx)
	case bankActionWriteFetched:
		return s.finalizeWriteFetchedTrans(trans)
	default:
		panic("cannot handle trans bank action")
	}
}

func (s *bankStage) finalizeReadHitTrans(
	trans *transactionState, postCoalesceIdx int,
) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	data, err := s.cache.storage.Read(
		nextBlock.CacheAddress, trans.ReadAccessByteSize)
	if err != nil {
		panic(err)
	}

	nextBlock.ReadCount--

	for _, preIdx := range trans.PreCoalesceTransIdxs {
		t := &next.Transactions[preIdx]
		offset := t.ReadAddress - nextBlock.Tag
		t.Data = data[offset : offset+t.ReadAccessByteSize]
		t.Done = true
	}

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Elements = bankPostBuf.Elements[1:]
	s.removeTransaction(trans)

	tracing.EndTask(trans.ID, s.cache.comp)

	return true
}

func (s *bankStage) finalizeWriteTrans(
	trans *transactionState, postCoalesceIdx int,
) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]
	blockSize := 1 << s.cache.GetSpec().Log2BlockSize

	data, err := s.cache.storage.Read(nextBlock.CacheAddress, uint64(blockSize))
	if err != nil {
		panic(err)
	}

	offset := trans.WriteAddress - nextBlock.Tag

	for i := 0; i < len(trans.WriteData); i++ {
		if trans.WriteDirtyMask[i] {
			data[offset+uint64(i)] = trans.WriteData[i]
		}
	}

	err = s.cache.storage.Write(nextBlock.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.WriteDirtyMask
	nextBlock.IsLocked = false

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Elements = bankPostBuf.Elements[1:]

	if s.cache.writePolicy.NeedsDualCompletion() {
		trans.BankDone = true

		if trans.BottomWriteDone {
			s.removeTransaction(trans)
			tracing.EndTask(trans.ID, s.cache.comp)
		}
	} else {
		tracing.EndTask(trans.ID, s.cache.comp)
	}

	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(trans *transactionState) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	err := s.cache.storage.Write(nextBlock.CacheAddress, trans.Data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.WriteFetchedDirtyMask
	nextBlock.IsLocked = false

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Elements = bankPostBuf.Elements[1:]

	return true
}

func (s *bankStage) removeTransaction(trans *transactionState) {
	trans.Removed = true
}
