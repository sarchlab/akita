package datamover

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// dataTransferMW handles data read/write operations between source and
// destination ports.
type dataTransferMW struct {
	comp *Comp
}

func (m *dataTransferMW) portFor(side datamoverprotocol.Side) messaging.Port {
	switch side {
	case datamoverprotocol.SideInside:
		return m.comp.Ports.Inside
	case datamoverprotocol.SideOutside:
		return m.comp.Ports.Outside
	default:
		panic("invalid memory side")
	}
}

func (m *dataTransferMW) mapperFor(side datamoverprotocol.Side) mem.AddressMapper {
	switch side {
	case datamoverprotocol.SideInside:
		return m.comp.Spec.Inside
	case datamoverprotocol.SideOutside:
		return m.comp.Spec.Outside
	default:
		panic("invalid memory side")
	}
}

// Handle runs data transfer stages. Paused data movers make no progress;
// draining data movers continue to let the current transaction
// complete so a drain can converge.
func (m *dataTransferMW) Handle(_ timing.Event) bool {
	if m.comp.State.ControlState == controlStatePaused {
		return false
	}

	inside, outside := m.comp.Ports.Inside, m.comp.Ports.Outside
	madeProgress := m.processWriteDone(inside) || m.processWriteDone(outside)
	madeProgress = m.writeToDst() || madeProgress
	madeProgress = (m.processDataReady(inside) || m.processDataReady(outside)) || madeProgress
	madeProgress = m.readFromSrc() || madeProgress

	return madeProgress
}

// All copies share the read window. As in the serial mover, a source chunk
// may start while its first byte fits; the reservation lasts until its last
// byte is written. Issue reads in admission order.
func (m *dataTransferMW) readFromSrc() bool {
	transfers := m.comp.State.Transactions
	var used uint64
	var trans *dataMoverTransactionState
	for i := range transfers {
		t := &transfers[i]
		issued := t.NextReadAddr - t.SrcAddress
		used += issued - t.Buffer.Offset
		if trans == nil && issued < t.ByteSize {
			trans = t
		}
	}
	if trans == nil || used >= m.comp.Spec.BufferSize {
		return false
	}
	addr := trans.NextReadAddr
	width := min(trans.Buffer.Granularity, trans.ByteSize-(addr-trans.SrcAddress))
	srcP := m.portFor(trans.SrcSide)

	req := messaging.Msg{Payload: memprotocol.ReadReq{
		Address: addr,

		AccessByteSize: width,
		PID:            0},
		ID: m.comp.NewID(),

		Src: srcP.AsRemote(),
		Dst: m.mapperFor(trans.SrcSide).Find(addr),

		TrafficBytes: 12,
	}

	if !srcP.CanSend() {
		return false
	}

	taskID := m.comp.TaskTracker().StartReqOut(m.comp, req, trans.RecvTaskID)
	srcP.Send(req)

	trans.NextReadAddr += width
	trans.PendingRead[req.ID] = pendingReadState{TaskID: taskID, Offset: addr - trans.SrcAddress}

	return true
}

// Match the returned request ID to its owner. Unmatched responses can
// arrive after Reset, including on a side now used in the other direction.
func (m *dataTransferMW) processDataReady(port messaging.Port) bool {
	rsp, present := port.PeekIncoming()
	data, ok := rsp.Payload.(memprotocol.DataReadyRsp)
	if !present || !ok {
		return false
	}
	port.RetrieveIncoming()
	for i := range m.comp.State.Transactions {
		trans := &m.comp.State.Transactions[i]
		req, ok := trans.PendingRead[rsp.RspTo]
		if !ok {
			continue
		}
		bufferAddData(&trans.Buffer, req.Offset, data.Data)
		delete(trans.PendingRead, rsp.RspTo)
		m.comp.TaskTracker().AddMilestone(tasks.Milestone{
			TaskID: trans.RecvTaskID, Kind: tasks.MilestoneKindData, What: port.Name(),
		})
		m.comp.TaskTracker().EndReqOut(req.TaskID)
		break
	}
	return true
}

// writeToDst sends data to destination.
func (m *dataTransferMW) writeToDst() bool {
	transfers := m.comp.State.Transactions
	if len(transfers) == 0 {
		return false
	}
	dstP := m.portFor(transfers[0].DstSide)
	if !dstP.CanSend() {
		return false
	}
	for i := range transfers {
		trans := &transfers[i]
		offset := trans.NextWriteAddr - trans.DstAddress
		if offset >= trans.ByteSize {
			continue
		}
		width := min(resolveByteGranularity(m.comp.Spec, trans.DstSide), trans.ByteSize-offset)
		data, ready := bufferExtractData(&trans.Buffer, offset, width)
		if ready {
			m.sendWrite(dstP, trans, data)
			return true
		}
	}
	return false
}

func (m *dataTransferMW) sendWrite(dstP messaging.Port, trans *dataMoverTransactionState, data []byte) {
	req := messaging.Msg{Payload: memprotocol.WriteReq{
		Address: trans.NextWriteAddr,
		Data:    data,

		PID: 0},
		ID: m.comp.NewID(),

		Src: dstP.AsRemote(),
		Dst: m.mapperFor(trans.DstSide).Find(trans.NextWriteAddr),

		TrafficBytes: len(data) + 12,
	}

	taskID := m.comp.TaskTracker().StartReqOut(m.comp, req, trans.RecvTaskID)
	dstP.Send(req)

	trans.NextWriteAddr += uint64(len(data))
	trans.PendingWrite[req.ID] = taskID
	written := trans.NextWriteAddr - trans.DstAddress
	if written == trans.ByteSize {
		// Release the final source chunk even when it is shorter than the granularity.
		trans.Buffer.Chunks = nil
		trans.Buffer.Offset = written
	} else {
		bufferMoveOffsetForwardTo(&trans.Buffer, written)
	}
}

func (m *dataTransferMW) processWriteDone(port messaging.Port) bool {
	rsp, present := port.PeekIncoming()
	_, ok := rsp.Payload.(memprotocol.WriteDoneRsp)
	if !present || !ok {
		return false
	}
	port.RetrieveIncoming()
	for i := range m.comp.State.Transactions {
		trans := &m.comp.State.Transactions[i]
		taskID, ok := trans.PendingWrite[rsp.RspTo]
		if !ok {
			continue
		}
		delete(trans.PendingWrite, rsp.RspTo)
		m.comp.TaskTracker().AddMilestone(tasks.Milestone{
			TaskID: trans.RecvTaskID, Kind: tasks.MilestoneKindSubTask, What: port.Name(),
		})
		m.comp.TaskTracker().EndReqOut(taskID)
		break
	}
	return true
}
