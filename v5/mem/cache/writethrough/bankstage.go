package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type bankTransaction struct {
	*transaction
}

func (t *bankTransaction) TaskID() string {
	return t.transaction.id
}

type bankStage struct {
	cache          *Comp
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
		transaction: item.(*transaction),
	})
	s.cache.bankBufs[s.bankID].Pop()

	return true
}

func (s *bankStage) finalizeTrans() bool {
	item := s.postPipelineBuf.Peek()
	if item == nil {
		return false
	}

	trans := item.(*bankTransaction).transaction

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

func (s *bankStage) finalizeReadHitTrans(trans *transaction) bool {
	block := trans.block
	readPayload := sim.MsgPayload[mem.ReadReqPayload](trans.read)

	data, err := s.cache.storage.Read(
		block.CacheAddress, readPayload.AccessByteSize)
	if err != nil {
		panic(err)
	}

	block.ReadCount--

	for _, t := range trans.preCoalesceTransactions {
		tReadPayload := sim.MsgPayload[mem.ReadReqPayload](t.read)
		offset := tReadPayload.Address - block.Tag
		t.data = data[offset : offset+tReadPayload.AccessByteSize]
		t.done = true
	}

	s.removeTransaction(trans)
	s.postPipelineBuf.Pop()

	tracing.EndTask(trans.id, s.cache)

	return true
}

func (s *bankStage) finalizeWriteTrans(trans *transaction) bool {
	writePayload := sim.MsgPayload[mem.WriteReqPayload](trans.write)
	block := trans.block
	blockSize := 1 << s.cache.log2BlockSize

	data, err := s.cache.storage.Read(block.CacheAddress, uint64(blockSize))
	if err != nil {
		panic(err)
	}

	offset := writePayload.Address - block.Tag

	for i := 0; i < len(writePayload.Data); i++ {
		if writePayload.DirtyMask[i] {
			data[offset+uint64(i)] = writePayload.Data[i]
		}
	}

	err = s.cache.storage.Write(block.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = writePayload.DirtyMask
	block.IsLocked = false

	s.postPipelineBuf.Pop()

	tracing.EndTask(trans.id, s.cache)

	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(trans *transaction) bool {
	block := trans.block

	err := s.cache.storage.Write(block.CacheAddress, trans.data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = trans.writeFetchedDirtyMask
	block.IsLocked = false

	s.postPipelineBuf.Pop()

	return true
}

func (s *bankStage) removeTransaction(trans *transaction) {
	for i, t := range s.cache.postCoalesceTransactions {
		if t == trans {
			s.cache.postCoalesceTransactions = append(
				s.cache.postCoalesceTransactions[:i],
				s.cache.postCoalesceTransactions[i+1:]...)

			return
		}
	}
}
