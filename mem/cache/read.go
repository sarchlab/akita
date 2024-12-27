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
	for i := 0; i < s.NumReqPerCycle; i++ {
		req := s.topPort.PeekIncoming()
		if req == nil {
			break
		}

		read, ok := req.(mem.ReadReq)
		if !ok {
			break
		}

		inMSHR := s.MSHR.Lookup(read.PID, read.Address)
		if inMSHR {
			madeProgress = s.handleMSHRHit(read) || madeProgress
			continue
		}

		block, ok := s.Tags.Lookup(read.PID, read.Address)
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
	s.MSHR.AddReqToEntry(read)
	s.topPort.RetrieveIncoming()

	return true
}

func (s *defaultReadStrategy) HandleReadHit(
	req mem.ReadReq,
	b tagging.Block,
) (madeProgress bool) {
	if !s.TopDownPreStorageBuffer.CanPush() {
		return false
	}

	transaction := &transaction{
		req:   req,
		setID: b.SetID,
		wayID: b.WayID,
	}
	s.Transactions = append(s.Transactions, transaction)

	b.IsLocked = true
	s.Tags.Visit(b)
	s.TopDownPreStorageBuffer.Push(transaction)
	s.tagCacheHit(transaction)
	s.topPort.RetrieveIncoming()

	return true
}

func (s *defaultReadStrategy) HandleReadMiss(
	req mem.ReadReq,
) (madeProgress bool) {
	if s.MSHR.IsFull() {
		return false
	}

	if !s.bottomPort.CanSend() {
		return false
	}

	victim, ok := s.VictimFinder.FindVictim(s.Tags, req.Address)
	if !ok || victim.IsLocked {
		return false
	}

	if victim.IsDirty && !s.EvictQueue.CanPush() {
		return false
	}

	if !s.bottomPort.CanSend() {
		return false
	}

	transaction := &transaction{
		req:   req,
		setID: victim.SetID,
		wayID: victim.WayID,
	}
	s.Transactions = append(s.Transactions, transaction)

	if victim.IsDirty {
		s.EvictQueue.Push(transaction)
	}

	alignedAddr := s.alignAddrToBlock(req.Address)
	blockSize := 1 << s.Log2BlockSize
	downReq := mem.ReadReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: s.bottomPort.AsRemote(),
		},
		PID:            req.PID,
		Address:        alignedAddr,
		AccessByteSize: uint64(blockSize),
	}

	victim.IsLocked = true
	s.Tags.Visit(victim)
	s.MSHR.AddEntry(downReq)
	s.MSHR.AddReqToEntry(req)
	s.topPort.RetrieveIncoming()
	s.bottomPort.Send(downReq)
	s.tagCacheMiss(transaction)

	return true
}

func (s *defaultReadStrategy) alignAddrToBlock(addr uint64) uint64 {
	return addr & ^((uint64(1) << s.Log2BlockSize) - 1)
}
