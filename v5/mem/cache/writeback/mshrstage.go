package writeback

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type mshrStage struct {
	cache *middleware

	// The transaction that carries MSHR data/transaction pointers
	hasProcessingTrans     bool
	processingTrans        *transactionState
	processingTransList    []*transactionState
	processingData         []byte
}

func (s *mshrStage) Tick() bool {
	if s.hasProcessingTrans {
		return s.processOneReq()
	}

	item := s.cache.mshrStageBuffer.Pop()
	if item == nil {
		return false
	}

	trans := item.(*transactionState)
	s.hasProcessingTrans = true
	s.processingTrans = trans
	s.processingTransList = trans.mshrTransactions
	s.processingData = trans.mshrData

	return s.processOneReq()
}

func (s *mshrStage) Reset() {
	s.hasProcessingTrans = false
	s.processingTrans = nil
	s.processingTransList = nil
	s.processingData = nil
	s.cache.mshrStageBuffer.Clear()
}

func (s *mshrStage) processOneReq() bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	if len(s.processingTransList) == 0 {
		s.hasProcessingTrans = false
		return true
	}

	trans := s.processingTransList[0]

	transactionPresent := s.findTransaction(trans)

	if transactionPresent {
		s.removeTransaction(trans)

		if trans.read != nil {
			s.respondRead(trans.read, s.processingData)
		} else {
			s.respondWrite(trans.write)
		}

		s.processingTransList = s.processingTransList[1:]
		if len(s.processingTransList) == 0 {
			s.hasProcessingTrans = false
		}

		return true
	}

	s.processingTransList = s.processingTransList[1:]
	if len(s.processingTransList) == 0 {
		s.hasProcessingTrans = false
	}

	return true
}

func (s *mshrStage) respondRead(
	read *mem.ReadReq,
	data []byte,
) {
	_, offset := getCacheLineID(read.Address, s.cache.log2BlockSize)
	respondData := data[offset : offset+read.AccessByteSize]
	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort.AsRemote()
	dataReady.Dst = read.Src
	dataReady.RspTo = read.ID
	dataReady.Data = respondData
	dataReady.TrafficBytes = len(respondData) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(read, s.cache)
}

func (s *mshrStage) respondWrite(write *mem.WriteReq) {
	writeDoneRsp := &mem.WriteDoneRsp{}
	writeDoneRsp.ID = sim.GetIDGenerator().Generate()
	writeDoneRsp.Src = s.cache.topPort.AsRemote()
	writeDoneRsp.Dst = write.Src
	writeDoneRsp.RspTo = write.ID
	writeDoneRsp.TrafficBytes = 4
	writeDoneRsp.TrafficClass = "mem.WriteDoneRsp"
	s.cache.topPort.Send(writeDoneRsp)

	tracing.TraceReqComplete(write, s.cache)
}

func (s *mshrStage) removeTransaction(trans *transactionState) {
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

func (s *mshrStage) findTransaction(trans *transactionState) bool {
	for _, t := range s.cache.inFlightTransactions {
		if trans == t {
			return true
		}
	}

	return false
}
