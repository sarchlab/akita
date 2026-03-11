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
