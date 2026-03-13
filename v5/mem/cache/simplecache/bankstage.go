package simplecache

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

	item := bankBuf.Peek()
	if item == nil {
		return false
	}

	bankPipeline := &next.BankPipelines[s.bankID]
	if !bankPipeline.CanAccept() {
		return false
	}

	transIdx := item.(int)
	bankPipeline.Accept(transIdx)
	bankBuf.Pop()

	return true
}

func (s *bankStage) finalizeTrans() bool {
	next := s.cache.comp.GetNextState()
	bankPostBuf := &next.BankPostBufs[s.bankID]

	item := bankPostBuf.Peek()
	if item == nil {
		return false
	}

	transIdx := item.(int)
	trans := s.cache.postCoalesceTransactions[transIdx]

	switch trans.bankAction {
	case bankActionReadHit:
		return s.finalizeReadHitTrans(trans)
	case bankActionWrite:
		return s.finalizeWriteTrans(trans)
	case bankActionWriteFetched:
		return s.finalizeWriteFetchedTrans(trans)
	default:
		panic("cannot handle trans bank action")
	}
}

func (s *bankStage) finalizeReadHitTrans(trans *transactionState) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	data, err := s.cache.storage.Read(
		nextBlock.CacheAddress, trans.read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	nextBlock.ReadCount--

	for _, t := range trans.preCoalesceTransactions {
		offset := t.read.Address - nextBlock.Tag
		t.data = data[offset : offset+t.read.AccessByteSize]
		t.done = true
	}

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Pop()
	s.removeTransaction(trans)

	tracing.EndTask(trans.id, s.cache.comp)

	return true
}

func (s *bankStage) finalizeWriteTrans(trans *transactionState) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]
	blockSize := 1 << s.cache.GetSpec().Log2BlockSize

	data, err := s.cache.storage.Read(nextBlock.CacheAddress, uint64(blockSize))
	if err != nil {
		panic(err)
	}

	offset := trans.write.Address - nextBlock.Tag

	for i := 0; i < len(trans.write.Data); i++ {
		if trans.write.DirtyMask[i] {
			data[offset+uint64(i)] = trans.write.Data[i]
		}
	}

	err = s.cache.storage.Write(nextBlock.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.write.DirtyMask
	nextBlock.IsLocked = false

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Pop()

	if s.cache.writePolicy.NeedsDualCompletion() {
		trans.bankDone = true

		if trans.bottomWriteDone {
			s.removeTransaction(trans)
			tracing.EndTask(trans.id, s.cache.comp)
		}
	} else {
		tracing.EndTask(trans.id, s.cache.comp)
	}

	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(trans *transactionState) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	err := s.cache.storage.Write(nextBlock.CacheAddress, trans.data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.writeFetchedDirtyMask
	nextBlock.IsLocked = false

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Pop()

	return true
}

func (s *bankStage) removeTransaction(trans *transactionState) {
	for i, t := range s.cache.postCoalesceTransactions {
		if t == trans {
			s.cache.postCoalesceTransactions[i] = nil

			return
		}
	}
}

