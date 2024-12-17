package cache

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/mshr"
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

		entry := s.MSHR.Query(read.PID, read.Address)
		if entry != nil {
			madeProgress = s.handleMSHRHit(read, entry) || madeProgress
			continue
		}

		block, ok := s.Tags.Lookup(read.PID, read.Address)
		if ok {
			madeProgress = s.HandleReadHit(read, &block) || madeProgress
			continue
		}

		madeProgress = s.HandleReadMiss(read) || madeProgress
	}

	return madeProgress
}

func (s *defaultReadStrategy) handleMSHRHit(
	read mem.ReadReq,
	entry *mshr.MSHREntry,
) (madeProgress bool) {
	entry.Requests = append(entry.Requests, read)
	s.Transactions = append(s.Transactions, &transaction{
		req:       read,
		mshrEntry: entry,
	})

	s.topPort.RetrieveIncoming()

	return true
}

func (s *defaultReadStrategy) HandleReadHit(
	req mem.ReadReq,
	b *tagging.Block,
) (madeProgress bool) {
	panic("read hit not implemented")
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

	victim := s.Tags.FindVictim(req.Address)
	if victim == nil || victim.IsLocked {
		return false
	}

	if victim.IsDirty && !s.EvictQueue.CanPush() {
		return false
	}

	transaction := &transaction{
		req:   req,
		block: victim,
	}
	s.Transactions = append(s.Transactions, transaction)

	if victim.IsDirty {
		s.EvictQueue.Push(transaction)
	}

	mshrEntry := s.MSHR.Add(req.PID, req.Address)
	mshrEntry.Requests = append(mshrEntry.Requests, req)
	transaction.mshrEntry = mshrEntry

	clAddr := s.alignAddrToBlock(req.Address)
	blockSize := 1 << s.Log2BlockSize
	readReq := mem.ReadReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: s.bottomPort.AsRemote(),
		},
		PID:            req.PID,
		Address:        clAddr,
		AccessByteSize: uint64(blockSize),
	}
	s.bottomPort.Send(readReq)

	victim.IsLocked = true
	s.Tags.Visit(victim)
	s.topPort.RetrieveIncoming()

	return true
}

func (s *defaultReadStrategy) alignAddrToBlock(addr uint64) uint64 {
	return addr & ^((uint64(1) << s.Log2BlockSize) - 1)
}
