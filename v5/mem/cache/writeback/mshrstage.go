package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type mshrStage struct {
	cache *Comp

	processingMSHREntry *cache.MSHREntry
}

func (s *mshrStage) Tick() bool {
	if s.processingMSHREntry != nil {
		return s.processOneReq()
	}

	item := s.cache.mshrStageBuffer.Pop()
	if item == nil {
		return false
	}

	s.processingMSHREntry = item.(*cache.MSHREntry)

	return s.processOneReq()
}

func (s *mshrStage) Reset() {
	s.processingMSHREntry = nil
	s.cache.mshrStageBuffer.Clear()
}

func (s *mshrStage) processOneReq() bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	mshrEntry := s.processingMSHREntry
	trans := mshrEntry.Requests[0].(*transaction)

	transactionPresent := s.findTransaction(trans)

	if transactionPresent {
		s.removeTransaction(trans)

		if trans.read != nil {
			s.respondRead(trans.read, mshrEntry.Data)
		} else {
			s.respondWrite(trans.write)
		}

		mshrEntry.Requests = mshrEntry.Requests[1:]
		if len(mshrEntry.Requests) == 0 {
			s.processingMSHREntry = nil
		}

		return true
	}

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		s.processingMSHREntry = nil
	}

	return true
}

func (s *mshrStage) respondRead(
	readMsg *sim.GenericMsg,
	data []byte,
) {
	readPayload := sim.MsgPayload[mem.ReadReqPayload](readMsg)
	_, offset := getCacheLineID(readPayload.Address, s.cache.log2BlockSize)
	dataReady := mem.DataReadyRspBuilder{}.
		WithSrc(s.cache.topPort.AsRemote()).
		WithDst(readMsg.Src).
		WithRspTo(readMsg.ID).
		WithData(data[offset : offset+readPayload.AccessByteSize]).
		Build()
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(readMsg, s.cache)
}

func (s *mshrStage) respondWrite(writeMsg *sim.GenericMsg) {
	writeDoneRsp := mem.WriteDoneRspBuilder{}.
		WithSrc(s.cache.topPort.AsRemote()).
		WithDst(writeMsg.Src).
		WithRspTo(writeMsg.ID).
		Build()
	s.cache.topPort.Send(writeDoneRsp)

	tracing.TraceReqComplete(writeMsg, s.cache)
}

func (s *mshrStage) removeTransaction(trans *transaction) {
	for i, t := range s.cache.inFlightTransactions {
		if trans == t {
			s.cache.inFlightTransactions = append(
				(s.cache.inFlightTransactions)[:i],
				(s.cache.inFlightTransactions)[i+1:]...)

			return
		}
	}

	panic("transaction not found")
}

func (s *mshrStage) findTransaction(trans *transaction) bool {
	for _, t := range s.cache.inFlightTransactions {
		if trans == t {
			return true
		}
	}

	return false
}
