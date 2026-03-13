package writeback

import (
	"slices"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type mshrStage struct {
	cache *pipelineMW

	// The transaction that carries MSHR data/transaction indices
	hasProcessingTrans       bool
	processingTrans          *transactionState
	processingTransIndices   []int
	processingData           []byte
}

func (s *mshrStage) Tick() bool {
	if s.hasProcessingTrans {
		return s.processOneReq()
	}

	next := s.cache.comp.GetNextState()
	mshrBuf := &next.MSHRStageBuf

	item := mshrBuf.Pop()
	if item == nil {
		return false
	}

	transIdx := item.(int)
	trans := s.cache.inFlightTransactions[transIdx]
	s.hasProcessingTrans = true
	s.processingTrans = trans
	s.processingTransIndices = trans.MSHRTransactionIndices
	s.processingData = trans.MSHRData

	return s.processOneReq()
}

func (s *mshrStage) Reset() {
	s.hasProcessingTrans = false
	s.processingTrans = nil
	s.processingTransIndices = nil
	s.processingData = nil
	next := s.cache.comp.GetNextState()
	next.MSHRStageBuf.Clear()
}

func (s *mshrStage) processOneReq() bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	if len(s.processingTransIndices) == 0 {
		s.hasProcessingTrans = false
		return true
	}

	transIdx := s.processingTransIndices[0]

	// Verify the transaction is still in the inflight list
	var trans *transactionState
	if transIdx >= 0 && transIdx < len(s.cache.inFlightTransactions) {
		trans = s.cache.inFlightTransactions[transIdx]
	}

	transactionPresent := trans != nil && s.findTransaction(trans)

	spec := s.cache.comp.GetSpec()

	if transactionPresent {
		s.removeTransaction(trans)

		if trans.HasRead {
			s.respondRead(trans, s.processingData, spec.Log2BlockSize)
		} else {
			s.respondWrite(trans)
		}

		s.processingTransIndices = s.processingTransIndices[1:]
		if len(s.processingTransIndices) == 0 {
			s.hasProcessingTrans = false
		}

		return true
	}

	s.processingTransIndices = s.processingTransIndices[1:]
	if len(s.processingTransIndices) == 0 {
		s.hasProcessingTrans = false
	}

	return true
}

func (s *mshrStage) respondRead(
	trans *transactionState,
	data []byte,
	log2BlockSize uint64,
) {
	_, offset := getCacheLineID(trans.ReadAddress, log2BlockSize)
	respondData := data[offset : offset+trans.ReadAccessByteSize]
	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort.AsRemote()
	dataReady.Dst = trans.ReadMeta.Src
	dataReady.RspTo = trans.ReadMeta.ID
	dataReady.Data = respondData
	dataReady.TrafficBytes = len(respondData) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(&trans.ReadMeta, s.cache.comp)
}

func (s *mshrStage) respondWrite(trans *transactionState) {
	writeDoneRsp := &mem.WriteDoneRsp{}
	writeDoneRsp.ID = sim.GetIDGenerator().Generate()
	writeDoneRsp.Src = s.cache.topPort.AsRemote()
	writeDoneRsp.Dst = trans.WriteMeta.Src
	writeDoneRsp.RspTo = trans.WriteMeta.ID
	writeDoneRsp.TrafficBytes = 4
	writeDoneRsp.TrafficClass = "mem.WriteDoneRsp"
	s.cache.topPort.Send(writeDoneRsp)

	tracing.TraceReqComplete(&trans.WriteMeta, s.cache.comp)
}

func (s *mshrStage) removeTransaction(trans *transactionState) {
	for i, t := range s.cache.inFlightTransactions {
		if trans == t {
			s.cache.inFlightTransactions[i] = nil
			return
		}
	}

	panic("transaction not found")
}

func (s *mshrStage) findTransaction(trans *transactionState) bool {
	return slices.Contains(s.cache.inFlightTransactions, trans)
}
