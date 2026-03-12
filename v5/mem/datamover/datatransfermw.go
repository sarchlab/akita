package datamover

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// dataTransferMW handles data read/write operations between source and
// destination ports.
type dataTransferMW struct {
	comp *modeling.Component[Spec, State]
}

// NamedHookable delegation methods.

func (m *dataTransferMW) Name() string {
	return m.comp.Name()
}

func (m *dataTransferMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *dataTransferMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *dataTransferMW) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *dataTransferMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *dataTransferMW) insidePort() sim.Port {
	return m.comp.GetPortByName("Inside")
}

func (m *dataTransferMW) outsidePort() sim.Port {
	return m.comp.GetPortByName("Outside")
}

func (m *dataTransferMW) srcPort() sim.Port {
	cur := m.comp.GetState()
	switch cur.SrcSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataTransferMW) dstPort() sim.Port {
	cur := m.comp.GetState()
	switch cur.DstSide {
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
	cur := m.comp.GetState()
	switch cur.SrcSide {
	case "inside":
		return findPort(spec.InsideMapperKind, spec.InsideMapperPorts,
			spec.InsideMapperInterleavingSize, addr)
	case "outside":
		return findPort(spec.OutsideMapperKind, spec.OutsideMapperPorts,
			spec.OutsideMapperInterleavingSize, addr)
	default:
		log.Panicf("unknown src side %q", cur.SrcSide)
		return ""
	}
}

func (m *dataTransferMW) findDstPort(addr uint64) sim.RemotePort {
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()
	switch cur.DstSide {
	case "inside":
		return findPort(spec.InsideMapperKind, spec.InsideMapperPorts,
			spec.InsideMapperInterleavingSize, addr)
	case "outside":
		return findPort(spec.OutsideMapperKind, spec.OutsideMapperPorts,
			spec.OutsideMapperInterleavingSize, addr)
	default:
		log.Panicf("unknown dst side %q", cur.DstSide)
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
	cur := m.comp.GetState()
	if !cur.CurrentTransaction.Active {
		return false
	}

	curTrans := &cur.CurrentTransaction
	addr := alignAddress(curTrans.NextReadAddr, cur.SrcByteGranularity)

	spec := m.comp.GetSpec()
	bufEndAddr := cur.Buffer.Offset + spec.BufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := curTrans.SrcAddress + curTrans.ByteSize
	if addr >= transEndAddr {
		return false
	}

	srcP := m.srcPort()

	req := &mem.ReadReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = addr
	req.Src = srcP.AsRemote()
	req.Dst = m.findSrcPort(addr)
	req.AccessByteSize = cur.SrcByteGranularity
	req.PID = 0
	req.TrafficBytes = 12
	req.TrafficClass = "mem.ReadReq"

	err := srcP.Send(req)
	if err != nil {
		return false
	}

	next := m.comp.GetNextState()
	nextTrans := &next.CurrentTransaction
	nextTrans.NextReadAddr += cur.SrcByteGranularity
	nextTrans.PendingRead[req.ID] = pendingReadState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
	}

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(transactionAsMsg(nextTrans), m))

	return true
}

// processDataReadyFromSrc processes data ready from source.
func (m *dataTransferMW) processDataReadyFromSrc() bool {
	cur := m.comp.GetState()
	if !cur.CurrentTransaction.Active {
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

	curTrans := &cur.CurrentTransaction
	originalReq, ok := curTrans.PendingRead[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s", rsp.RspTo)
	}

	next := m.comp.GetNextState()
	nextTrans := &next.CurrentTransaction
	offset := originalReq.Address - curTrans.SrcAddress
	bufferAddData(&next.Buffer, offset, rsp.Data)

	delete(nextTrans.PendingRead, rsp.RspTo)
	srcP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := &mem.ReadReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(traceReq, m)

	return true
}

// writeToDst sends data to destination.
func (m *dataTransferMW) writeToDst() bool {
	cur := m.comp.GetState()
	if !cur.CurrentTransaction.Active {
		return false
	}

	curTrans := &cur.CurrentTransaction
	offset := curTrans.NextWriteAddr - curTrans.DstAddress
	data, ok := bufferExtractData(&cur.Buffer, offset, cur.DstByteGranularity)

	if !ok {
		return false
	}

	dstP := m.dstPort()

	req := &mem.WriteReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = curTrans.NextWriteAddr
	req.Data = data
	req.Src = dstP.AsRemote()
	req.Dst = m.findDstPort(curTrans.NextWriteAddr)
	req.PID = 0
	req.TrafficBytes = len(data) + 12
	req.TrafficClass = "mem.WriteReq"

	err := dstP.Send(req)
	if err != nil {
		return false
	}

	next := m.comp.GetNextState()
	nextTrans := &next.CurrentTransaction
	nextTrans.NextWriteAddr += cur.DstByteGranularity
	nextTrans.PendingWrite[req.ID] = pendingWriteState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
		Data:    data,
	}
	bufferMoveOffsetForwardTo(&next.Buffer, nextTrans.NextWriteAddr-curTrans.DstAddress)

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(transactionAsMsg(nextTrans), m))

	return true
}

// processWriteDoneFromDst processes write done from destination.
func (m *dataTransferMW) processWriteDoneFromDst() bool {
	cur := m.comp.GetState()
	if !cur.CurrentTransaction.Active {
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

	curTrans := &cur.CurrentTransaction
	originalReq, ok := curTrans.PendingWrite[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s", rsp.RspTo)
	}

	next := m.comp.GetNextState()
	nextTrans := &next.CurrentTransaction
	delete(nextTrans.PendingWrite, rsp.RspTo)
	dstP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := &mem.WriteReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(traceReq, m)

	return false
}
