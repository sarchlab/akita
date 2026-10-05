package datamover

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

// dataTransferMW handles data read/write operations between source and
// destination ports.
type dataTransferMW struct {
	comp *Comp

	// insideMapper and outsideMapper route each side's addresses to remote
	// ports. newMiddlewares takes them from Resources; they are not
	// checkpointed.
	insideMapper  mem.AddressToPortMapper
	outsideMapper mem.AddressToPortMapper
}

func (m *dataTransferMW) insidePort() messaging.Port {
	return m.comp.Ports.Inside
}

func (m *dataTransferMW) outsidePort() messaging.Port {
	return m.comp.Ports.Outside
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
	return m.mapperFor(m.comp.State.SrcSide).Find(addr)
}

func (m *dataTransferMW) findDstPort(addr uint64) messaging.RemotePort {
	return m.mapperFor(m.comp.State.DstSide).Find(addr)
}

// mapperFor returns the mapper for the "inside" or "outside" side.
func (m *dataTransferMW) mapperFor(side string) mem.AddressToPortMapper {
	var mapper mem.AddressToPortMapper

	switch side {
	case "inside":
		mapper = m.insideMapper
	case "outside":
		mapper = m.outsideMapper
	default:
		log.Panicf("unknown side %q", side)
	}

	if mapper == nil {
		log.Panicf("datamover: no mapper for the %s side; "+
			"set Resources.InsideMapper and Resources.OutsideMapper", side)
	}

	return mapper
}

// Handle runs data transfer stages. Paused data movers make no progress;
// draining data movers continue to let the current transaction
// complete so a drain can converge.
func (m *dataTransferMW) Handle(_ timing.Event) bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
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

	spec := m.comp.Spec
	// The buffer is indexed in transaction-relative space (offsets measured from
	// SrcAddress), and Buffer.Offset slides in that same relative space as data
	// is written out. The read-window check must therefore use the relative
	// address; comparing the absolute addr would wrongly reject every read of a
	// transaction whose SrcAddress is at or beyond BufferSize.
	relAddr := addr - trans.SrcAddress
	bufEndAddr := state.Buffer.Offset + spec.BufferSize
	if relAddr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.SrcAddress + trans.ByteSize
	if addr >= transEndAddr {
		return false
	}

	srcP := m.srcPort()

	req := messaging.Msg{Payload: memprotocol.ReadReq{
		Address: addr,

		AccessByteSize: state.SrcByteGranularity,
		PID:            0},
		ID: m.comp.NewID(),

		Src: srcP.AsRemote(),
		Dst: m.findSrcPort(addr),

		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	if !srcP.CanSend() {
		return false
	}

	srcP.Send(req)

	trans.NextReadAddr += state.SrcByteGranularity
	trans.PendingRead[req.ID] = pendingReadState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Payload.(memprotocol.ReadReq).Address,
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
	rspI, ok := srcP.PeekIncoming()
	if !ok {
		return false
	}
	_, ok = rspI.Payload.(memprotocol.DataReadyRsp)
	rsp := rspI
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
	bufferAddData(&state.Buffer, offset, rsp.Payload.(memprotocol.DataReadyRsp).Data)

	delete(trans.PendingRead, rsp.RspTo)
	srcP.RetrieveIncoming()

	// Charge the in-flight src-read wait to the req_in opened for this move
	// transaction (same key used as the parent of the read req_out's
	// TraceReqInitiate), so the dominant latency of a move is attributed.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(transactionAsMsg(trans), m.comp),
		Kind:   tracing.MilestoneKindData,
		What:   m.srcPort().Name(),
	})

	// Create a temporary msg for tracing
	traceReq := messaging.Msg{Payload: memprotocol.ReadReq{},
		ID:  originalReq.ID,
		Src: originalReq.Src,
		Dst: originalReq.Dst}

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

	req := messaging.Msg{Payload: memprotocol.WriteReq{
		Address: trans.NextWriteAddr,
		Data:    data,

		PID: 0},
		ID: m.comp.NewID(),

		Src: dstP.AsRemote(),
		Dst: m.findDstPort(trans.NextWriteAddr),

		TrafficBytes: len(data) + 12,
		TrafficClass: "memprotocol.WriteReq"}

	if !dstP.CanSend() {
		return false
	}

	dstP.Send(req)

	trans.NextWriteAddr += state.DstByteGranularity
	trans.PendingWrite[req.ID] = pendingWriteState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Payload.(memprotocol.WriteReq).Address,
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
	rspI, ok := dstP.PeekIncoming()
	if !ok {
		return false
	}
	_, ok = rspI.Payload.(memprotocol.WriteDoneRsp)
	rsp := rspI
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

	// Charge the in-flight dst-write wait to the req_in opened for this move
	// transaction (same key used as the parent of the write req_out's
	// TraceReqInitiate), so the dominant latency of a move is attributed.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(transactionAsMsg(trans), m.comp),
		Kind:   tracing.MilestoneKindSubTask,
		What:   m.dstPort().Name(),
	})

	// Create a temporary msg for tracing
	traceReq := messaging.Msg{Payload: memprotocol.WriteReq{},
		ID:  originalReq.ID,
		Src: originalReq.Src,
		Dst: originalReq.Dst}

	tracing.TraceReqFinalize(m.comp, traceReq)

	// Processing a write ack is real progress: the component must tick again
	// so the remaining acks drain and the transaction can finish (which now
	// waits for every write to be acknowledged).
	return true
}
