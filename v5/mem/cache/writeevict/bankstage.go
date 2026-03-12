package writeevict

import (
	"github.com/sarchlab/akita/v5/tracing"
)

type bankTransaction struct {
	*transactionState
}

func (t *bankTransaction) TaskID() string {
	return t.transactionState.id
}

type bankStage struct {
	cache          *pipelineMW
	bankID         int
	numReqPerCycle int
}

func (s *bankStage) Reset() {
	next := s.cache.comp.GetNextState()
	next.BankPostPipelineBufIndices[s.bankID].Indices = nil
	next.BankPipelineStages[s.bankID].Stages = nil
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
	spec := s.cache.GetSpec()

	return bankPipelineTick(
		&next.BankPipelineStages[s.bankID].Stages,
		&next.BankPostPipelineBufIndices[s.bankID].Indices,
		s.numReqPerCycle,
		spec.BankLatency,
	)
}

func (s *bankStage) extractFromBuf() bool {
	item := s.cache.bankBufAdapters[s.bankID].Peek()
	if item == nil {
		return false
	}

	next := s.cache.comp.GetNextState()
	stages := next.BankPipelineStages[s.bankID].Stages

	if !bankPipelineCanAccept(stages, s.numReqPerCycle) {
		return false
	}

	trans := item.(*transactionState)
	transIdx := s.findPostCoalesceIdx(trans)
	bankPipelineAccept(
		&next.BankPipelineStages[s.bankID].Stages,
		s.numReqPerCycle,
		transIdx,
	)
	s.cache.bankBufAdapters[s.bankID].Pop()

	return true
}

func (s *bankStage) finalizeTrans() bool {
	item := s.cache.bankPostBufAdapters[s.bankID].Peek()
	if item == nil {
		return false
	}

	trans := item.(*bankTransaction).transactionState

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

	s.cache.bankPostBufAdapters[s.bankID].Pop()
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

	s.cache.bankPostBufAdapters[s.bankID].Pop()

	tracing.EndTask(trans.id, s.cache.comp)

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

	s.cache.bankPostBufAdapters[s.bankID].Pop()

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

func (s *bankStage) findPostCoalesceIdx(
	trans *transactionState,
) int {
	for i, t := range s.cache.postCoalesceTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in postCoalesceTransactions")
}
