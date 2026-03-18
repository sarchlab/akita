package datamover

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// dataTransferMW handles data read/write operations between source and
// destination ports.
type dataTransferMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *dataTransferMW) insidePort() sim.Port {
	return m.comp.GetPortByName("Inside")
}

func (m *dataTransferMW) outsidePort() sim.Port {
	return m.comp.GetPortByName("Outside")
}

func (m *dataTransferMW) srcPort() sim.Port {
	state := m.comp.GetNextState()
	switch state.SrcSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataTransferMW) dstPort() sim.Port {
	state := m.comp.GetNextState()
	switch state.DstSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataTransferMW) findSrcPort(addr uint64) sim.RemotePort {
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()
	switch state.SrcSide {
	case "inside":
		return findPort(spec.InsideMapperKind, spec.InsideMapperPorts,
			spec.InsideMapperInterleavingSize, addr)
	case "outside":
		return findPort(spec.OutsideMapperKind, spec.OutsideMapperPorts,
			spec.OutsideMapperInterleavingSize, addr)
	default:
		log.Panicf("unknown src side %q", state.SrcSide)
		return ""
	}
}

func (m *dataTransferMW) findDstPort(addr uint64) sim.RemotePort {
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()
	switch state.DstSide {
	case "inside":
		return findPort(spec.InsideMapperKind, spec.InsideMapperPorts,
			spec.InsideMapperInterleavingSize, addr)
	case "outside":
		return findPort(spec.OutsideMapperKind, spec.OutsideMapperPorts,
			spec.OutsideMapperInterleavingSize, addr)
	default:
		log.Panicf("unknown dst side %q", state.DstSide)
		return ""
	}
}

// Tick runs data transfer stages.
func (m *dataTransferMW) Tick() bool {
	madeProgress := false

	madeProgress = m.processWriteDoneFromDst() || madeProgress
	madeProgress = m.writeToDst() || madeProgress
	madeProgress = m.processDataReadyFromSrc() || madeProgress
	madeProgress = m.readFromSrc() || madeProgress

	return madeProgress
}

// readFromSrc reads data from source.
func (m *dataTransferMW) readFromSrc() bool {
	state := m.comp.GetNextState()
	if !state.CurrentTransaction.Active {
		return false
	}

	trans := &state.CurrentTransaction
	addr := alignAddress(trans.NextReadAddr, state.SrcByteGranularity)

	spec := m.comp.GetSpec()
	bufEndAddr := state.Buffer.Offset + spec.BufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.SrcAddress + trans.ByteSize
	if addr >= transEndAddr {
		return false
	}

	srcP := m.srcPort()

	req := &mem.ReadReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = addr
	req.Src = srcP.AsRemote()
	req.Dst = m.findSrcPort(addr)
	req.AccessByteSize = state.SrcByteGranularity
	req.PID = 0
	req.TrafficBytes = 12
	req.TrafficClass = "mem.ReadReq"

	err := srcP.Send(req)
	if err != nil {
		return false
	}

	trans.NextReadAddr += state.SrcByteGranularity
	trans.PendingRead[req.ID] = pendingReadState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
	}

	tracing.TraceReqInitiate(req, m.comp,
		tracing.MsgIDAtReceiver(transactionAsMsg(trans), m.comp))

	return true
}

// processDataReadyFromSrc processes data ready from source.
func (m *dataTransferMW) processDataReadyFromSrc() bool {
	state := m.comp.GetNextState()
	if !state.CurrentTransaction.Active {
		return false
	}

	srcP := m.srcPort()
	rspI := srcP.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(*mem.DataReadyRsp)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	trans := &state.CurrentTransaction
	originalReq, ok := trans.PendingRead[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %d", rsp.RspTo)
	}

	offset := originalReq.Address - trans.SrcAddress
	bufferAddData(&state.Buffer, offset, rsp.Data)

	delete(trans.PendingRead, rsp.RspTo)
	srcP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := &mem.ReadReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(traceReq, m.comp)

	return true
}

// writeToDst sends data to destination.
func (m *dataTransferMW) writeToDst() bool {
	state := m.comp.GetNextState()
	if !state.CurrentTransaction.Active {
		return false
	}

	trans := &state.CurrentTransaction
	offset := trans.NextWriteAddr - trans.DstAddress
	data, ok := bufferExtractData(&state.Buffer, offset, state.DstByteGranularity)

	if !ok {
		return false
	}

	dstP := m.dstPort()

	req := &mem.WriteReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = trans.NextWriteAddr
	req.Data = data
	req.Src = dstP.AsRemote()
	req.Dst = m.findDstPort(trans.NextWriteAddr)
	req.PID = 0
	req.TrafficBytes = len(data) + 12
	req.TrafficClass = "mem.WriteReq"

	err := dstP.Send(req)
	if err != nil {
		return false
	}

	trans.NextWriteAddr += state.DstByteGranularity
	trans.PendingWrite[req.ID] = pendingWriteState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
		Data:    data,
	}
	bufferMoveOffsetForwardTo(&state.Buffer, trans.NextWriteAddr-trans.DstAddress)

	tracing.TraceReqInitiate(req, m.comp,
		tracing.MsgIDAtReceiver(transactionAsMsg(trans), m.comp))

	return true
}

// processWriteDoneFromDst processes write done from destination.
func (m *dataTransferMW) processWriteDoneFromDst() bool {
	state := m.comp.GetNextState()
	if !state.CurrentTransaction.Active {
		return false
	}

	dstP := m.dstPort()
	rspI := dstP.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(*mem.WriteDoneRsp)
	if !ok {
		return false
	}

	trans := &state.CurrentTransaction
	originalReq, ok := trans.PendingWrite[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %d", rsp.RspTo)
	}

	delete(trans.PendingWrite, rsp.RspTo)
	dstP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := &mem.WriteReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(traceReq, m.comp)

	return false
}
