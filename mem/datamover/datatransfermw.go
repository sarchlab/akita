package datamover

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// dataTransferMW handles data read/write operations between source and
	// destination ports.
	"github.com/sarchlab/akita/v5/messaging"
)

type dataTransferMW struct {
	comp *modeling.Component[Spec, State, modeling.None]
}

func (m *dataTransferMW) insidePort() messaging.Port {
	return m.comp.GetPortByName("Inside")
}

func (m *dataTransferMW) outsidePort() messaging.Port {
	return m.comp.GetPortByName("Outside")
}

func (m *dataTransferMW) srcPort() messaging.Port {
	state := &m.comp.State
	switch state.SrcSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataTransferMW) dstPort() messaging.Port {
	state := &m.comp.State
	switch state.DstSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataTransferMW) findSrcPort(addr uint64) messaging.RemotePort {
	spec := m.comp.Spec()
	state := &m.comp.State
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

func (m *dataTransferMW) findDstPort(addr uint64) messaging.RemotePort {
	spec := m.comp.Spec()
	state := &m.comp.State
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

// Tick runs data transfer stages. Paused data movers make no progress;
// draining data movers continue to let the current transaction
// complete so a drain can converge.
func (m *dataTransferMW) Tick() bool {
	if m.comp.State.ControlState == control.StatePaused {
		return false
	}

	madeProgress := false

	madeProgress = m.processWriteDoneFromDst() || madeProgress
	madeProgress = m.writeToDst() || madeProgress
	madeProgress = m.processDataReadyFromSrc() || madeProgress
	madeProgress = m.readFromSrc() || madeProgress

	return madeProgress
}

// readFromSrc reads data from source.
func (m *dataTransferMW) readFromSrc() bool {
	state := &m.comp.State
	if !state.CurrentTransaction.Active {
		return false
	}

	trans := &state.CurrentTransaction
	addr := alignAddress(trans.NextReadAddr, state.SrcByteGranularity)

	spec := m.comp.Spec()
	bufEndAddr := state.Buffer.Offset + spec.BufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.SrcAddress + trans.ByteSize
	if addr >= transEndAddr {
		return false
	}

	srcP := m.srcPort()

	req := mem.ReadReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Address = addr
	req.Src = srcP.AsRemote()
	req.Dst = m.findSrcPort(addr)
	req.AccessByteSize = state.SrcByteGranularity
	req.PID = 0
	req.TrafficBytes = 12
	req.TrafficClass = "mem.ReadReq"

	if !srcP.CanSend() {
		return false
	}

	srcP.Send(req)

	trans.NextReadAddr += state.SrcByteGranularity
	trans.PendingRead[req.ID] = pendingReadState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
	}

	tracing.TraceReqInitiate(m.comp, req,
		tracing.MsgIDAtReceiver(transactionAsMsg(trans), m.comp))

	return true
}

// processDataReadyFromSrc processes data ready from source.
func (m *dataTransferMW) processDataReadyFromSrc() bool {
	state := &m.comp.State
	if !state.CurrentTransaction.Active {
		return false
	}

	srcP := m.srcPort()
	rspI := srcP.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(mem.DataReadyRsp)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	trans := &state.CurrentTransaction
	originalReq, ok := trans.PendingRead[rsp.RspTo]
	if !ok {
		// Orphaned response: its read was discarded by a Reset issued while it
		// was in flight. Drop it rather than crash the current transaction.
		srcP.RetrieveIncoming()
		return true
	}

	offset := originalReq.Address - trans.SrcAddress
	bufferAddData(&state.Buffer, offset, rsp.Data)

	delete(trans.PendingRead, rsp.RspTo)
	srcP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := mem.ReadReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(m.comp, traceReq)

	return true
}

// writeToDst sends data to destination.
func (m *dataTransferMW) writeToDst() bool {
	state := &m.comp.State
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

	req := mem.WriteReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Address = trans.NextWriteAddr
	req.Data = data
	req.Src = dstP.AsRemote()
	req.Dst = m.findDstPort(trans.NextWriteAddr)
	req.PID = 0
	req.TrafficBytes = len(data) + 12
	req.TrafficClass = "mem.WriteReq"

	if !dstP.CanSend() {
		return false
	}

	dstP.Send(req)

	trans.NextWriteAddr += state.DstByteGranularity
	trans.PendingWrite[req.ID] = pendingWriteState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
		Data:    data,
	}
	bufferMoveOffsetForwardTo(&state.Buffer, trans.NextWriteAddr-trans.DstAddress)

	tracing.TraceReqInitiate(m.comp, req,
		tracing.MsgIDAtReceiver(transactionAsMsg(trans), m.comp))

	return true
}

// processWriteDoneFromDst processes write done from destination.
func (m *dataTransferMW) processWriteDoneFromDst() bool {
	state := &m.comp.State
	if !state.CurrentTransaction.Active {
		return false
	}

	dstP := m.dstPort()
	rspI := dstP.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(mem.WriteDoneRsp)
	if !ok {
		return false
	}

	trans := &state.CurrentTransaction
	originalReq, ok := trans.PendingWrite[rsp.RspTo]
	if !ok {
		// Orphaned ack: its write was discarded by a Reset issued while it was
		// in flight. Drop it rather than crash the current transaction.
		dstP.RetrieveIncoming()
		return true
	}

	delete(trans.PendingWrite, rsp.RspTo)
	dstP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := mem.WriteReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(m.comp, traceReq)

	// Processing a write ack is real progress: the component must tick again
	// so the remaining acks drain and the transaction can finish (which now
	// waits for every write to be acknowledged).
	return true
}
