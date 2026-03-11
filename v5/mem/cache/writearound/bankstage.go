package writearound

import (
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/tracing"
)

type bankTransaction struct {
	*transactionState
}

func (t *bankTransaction) TaskID() string {
	return t.transactionState.id
}

type bankStage struct {
	cache          *middleware
	bankID         int
	numReqPerCycle int

	pipeline        queueing.Pipeline
	postPipelineBuf queueing.Buffer
}

func (s *bankStage) Reset() {
	s.postPipelineBuf.Clear()
	s.pipeline.Clear()
}

func (s *bankStage) Tick() bool {
	madeProgress := false

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.pipeline.Tick() || madeProgress

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.extractFromBuf() || madeProgress
	}

	return madeProgress
}

func (s *bankStage) extractFromBuf() bool {
	item := s.cache.bankBufs[s.bankID].Peek()
	if item == nil {
		return false
	}

	if !s.pipeline.CanAccept() {
		return false
	}

	s.pipeline.Accept(&bankTransaction{
		transactionState: item.(*transactionState),
	})
	s.cache.bankBufs[s.bankID].Pop()

	return true
}

func (s *bankStage) finalizeTrans() bool {
	item := s.postPipelineBuf.Peek()
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
	block := &s.cache.directoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	data, err := s.cache.storage.Read(
		block.CacheAddress, trans.read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	block.ReadCount--

	for _, t := range trans.preCoalesceTransactions {
		offset := t.read.Address - block.Tag
		t.data = data[offset : offset+t.read.AccessByteSize]
		t.done = true
	}

	s.removeTransaction(trans)
	s.postPipelineBuf.Pop()

	tracing.EndTask(trans.id, s.cache)

	return true
}

func (s *bankStage) finalizeWriteTrans(trans *transactionState) bool {
	block := &s.cache.directoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]
	blockSize := 1 << s.cache.GetSpec().Log2BlockSize

	data, err := s.cache.storage.Read(block.CacheAddress, uint64(blockSize))
	if err != nil {
		panic(err)
	}

	offset := trans.write.Address - block.Tag

	for i := 0; i < len(trans.write.Data); i++ {
		if trans.write.DirtyMask[i] {
			data[offset+uint64(i)] = trans.write.Data[i]
		}
	}

	err = s.cache.storage.Write(block.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = trans.write.DirtyMask
	block.IsLocked = false

	s.postPipelineBuf.Pop()

	tracing.EndTask(trans.id, s.cache)

	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(trans *transactionState) bool {
	block := &s.cache.directoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	err := s.cache.storage.Write(block.CacheAddress, trans.data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = trans.writeFetchedDirtyMask
	block.IsLocked = false

	s.postPipelineBuf.Pop()

	return true
}

func (s *bankStage) removeTransaction(trans *transactionState) {
	for i, t := range s.cache.postCoalesceTransactions {
		if t == trans {
			s.cache.postCoalesceTransactions = append(
				s.cache.postCoalesceTransactions[:i],
				s.cache.postCoalesceTransactions[i+1:]...)

			return
		}
	}
}
