package cache

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// defaultReadStrategy is the default reading strategy.
type defaultReadStrategy struct {
	*Comp
}

func (s *defaultReadStrategy) Tick() bool {
	madeProgress := s.ParseTop()

	return madeProgress
}

func (s *defaultReadStrategy) ParseTop() (madeProgress bool) {
	for i := 0; i < s.numReqPerCycle; i++ {
		req := s.topPort.PeekIncoming()
		if req == nil {
			break
		}

		read, ok := req.(mem.ReadReq)
		if !ok {
			break
		}

		inMSHR := s.mshr.Lookup(read.PID, read.Address)
		if inMSHR {
			madeProgress = s.handleMSHRHit(read) || madeProgress
			continue
		}

		block, ok := s.tags.Lookup(read.PID, read.Address)
		if ok {
			madeProgress = s.HandleReadHit(read, block) || madeProgress
			continue
		}

		madeProgress = s.HandleReadMiss(read) || madeProgress
	}

	return madeProgress
}

func (s *defaultReadStrategy) handleMSHRHit(
	read mem.ReadReq,
) (madeProgress bool) {
	transaction := &transaction{
		req: read,
	}
	s.Transactions = append(s.Transactions, transaction)

	s.tagMSHRHit(transaction)
	s.mshr.AddReqToEntry(read)
	s.topPort.RetrieveIncoming()

	s.traceReqStart(read)

	return true
}

func (s *defaultReadStrategy) HandleReadHit(
	req mem.ReadReq,
	b tagging.Block,
) (madeProgress bool) {
	if !s.storageBottomUpBuf.CanPush() {
		return false
	}

	transaction := &transaction{
		transType: transactionTypeReadHit,
		req:       req,
		block:     b,
	}
	s.Transactions = append(s.Transactions, transaction)

	b.IsLocked = true
	s.tags.Visit(b)
	s.storageBottomUpBuf.Push(transaction)
	s.tagCacheHit(transaction)
	s.topPort.RetrieveIncoming()

	s.traceReqStart(req)

	return true
}

func (s *defaultReadStrategy) HandleReadMiss(
	req mem.ReadReq,
) (madeProgress bool) {
	if s.mshr.IsFull() {
		return false
	}

	victim, ok := s.victimFinder.FindVictim(s.tags, req.Address)
	if !ok || victim.IsLocked {
		return false
	}

	if victim.IsDirty && !s.storageTopDownBuf.CanPush() {
		return false
	}

	if !s.bottomInteractionBuf.CanPush() {
		return false
	}

	transaction := &transaction{
		transType: transactionTypeReadMiss,
		req:       req,
		block:     victim,
	}
	s.Transactions = append(s.Transactions, transaction)

	if victim.IsDirty {
		s.storageTopDownBuf.Push(transaction)
	}

	alignedAddr := getCacheLineAddr(req.Address, s.log2BlockSize)
	blockSize := 1 << s.log2BlockSize
	downReq := mem.ReadReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: s.bottomPort.AsRemote(),
			Dst: s.addressToDstTable.Find(alignedAddr),
		},
		PID:            req.PID,
		Address:        alignedAddr,
		AccessByteSize: uint64(blockSize),
	}

	transaction.reqToBottom = downReq
	victim.IsLocked = true

	s.tags.Visit(victim)
	s.mshr.AddEntry(downReq)
	s.mshr.AddReqToEntry(req)
	s.topPort.RetrieveIncoming()
	s.bottomInteractionBuf.Push(transaction)
	s.tagCacheMiss(transaction)
	s.traceReqToBottomStart(transaction)
	s.traceReqStart(req)

	return true
}
