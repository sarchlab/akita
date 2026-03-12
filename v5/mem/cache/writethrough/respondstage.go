package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type respondStage struct {
	cache *pipelineMW
}

func (s *respondStage) Tick() bool {
	if len(s.cache.transactions) == 0 {
		return false
	}

	for _, trans := range s.cache.transactions {
		if !trans.done {
			continue
		}

		if trans.read != nil {
			return s.respondReadTrans(trans)
		}

		return s.respondWriteTrans(trans)
	}

	return false
}

func (s *respondStage) respondReadTrans(trans *transactionState) bool {
	if !trans.done {
		return false
	}

	read := trans.read
	dr := &mem.DataReadyRsp{}
	dr.ID = sim.GetIDGenerator().Generate()
	dr.Src = s.cache.topPort.AsRemote()
	dr.Dst = read.Src
	dr.RspTo = read.ID
	dr.Data = trans.data
	dr.TrafficBytes = len(trans.data) + 4
	dr.TrafficClass = "rsp"

	err := s.cache.topPort.Send(dr)
	if err != nil {
		return false
	}

	s.removeTransaction(trans)

	tracing.TraceReqComplete(read, s.cache)

	return true
}

func (s *respondStage) respondWriteTrans(trans *transactionState) bool {
	if !trans.done {
		return false
	}

	write := trans.write
	done := &mem.WriteDoneRsp{}
	done.ID = sim.GetIDGenerator().Generate()
	done.Src = s.cache.topPort.AsRemote()
	done.Dst = write.Src
	done.RspTo = write.ID
	done.TrafficBytes = 4
	done.TrafficClass = "rsp"

	err := s.cache.topPort.Send(done)
	if err != nil {
		return false
	}

	s.removeTransaction(trans)

	tracing.TraceReqComplete(write, s.cache)

	return true
}

func (s *respondStage) removeTransaction(trans *transactionState) {
	for i, t := range s.cache.transactions {
		if t == trans {
			s.cache.transactions = append(s.cache.transactions[:i],
				s.cache.transactions[i+1:]...)

			return
		}
	}

	panic("not found")
}
