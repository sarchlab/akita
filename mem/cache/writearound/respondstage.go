package writearound

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/tracing"
)

type respondStage struct {
	cache *Comp
}

func (s *respondStage) Tick() bool {
	if len(s.cache.transactions) == 0 {
		return false
	}

	for _, trans := range s.cache.transactions {
		if !trans.done {
			continue
		}

		if trans.transactionType == transactionTypeRead {
			return s.respondReadTrans(trans)
		}

		return s.respondWriteTrans(trans)
	}

	return false
}

func (s *respondStage) respondReadTrans(trans *transaction) bool {
	if !trans.done {
		return false
	}

	read := trans.read
	dr := mem.DataReadyRsp{
		MsgMeta: modeling.MsgMeta{
			Src: s.cache.topPort.AsRemote(),
			Dst: read.Src,
		},
		Data:      trans.data,
		RespondTo: read.ID,
	}

	err := s.cache.topPort.Send(dr)
	if err != nil {
		return false
	}

	s.removeTransaction(trans)

	tracing.TraceReqComplete(read, s.cache)

	return true
}

func (s *respondStage) respondWriteTrans(trans *transaction) bool {
	if !trans.done {
		return false
	}

	write := trans.write
	done := mem.WriteDoneRsp{
		MsgMeta: modeling.MsgMeta{
			Src: s.cache.topPort.AsRemote(),
			Dst: write.Src,
		},
		RespondTo: write.ID,
	}

	err := s.cache.topPort.Send(done)
	if err != nil {
		return false
	}

	s.removeTransaction(trans)

	tracing.TraceReqComplete(write, s.cache)

	return true
}

func (s *respondStage) removeTransaction(trans *transaction) {
	for i, t := range s.cache.transactions {
		if t == trans {
			s.cache.transactions = append(s.cache.transactions[:i],
				s.cache.transactions[i+1:]...)
			return
		}
	}

	panic("not found")
}
