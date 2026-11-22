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

// All copies share the read window, counted in whole source units. As in the
// serial mover, a unit may start while its first byte fits; the reservation
// lasts until its last requested byte is written. Issue reads in admission
// order. The oldest copy that still has destination writes to issue may exceed
// the window for the units its next write needs, because an unaligned write can
// span more source units than the window holds and would otherwise never become
// ready. A copy with all writes issued and only acknowledgments pending no
// longer gates this exception, so a later copy may use it.
func (m *dataTransferMW) readFromSrc() bool {
	transfers := m.comp.State.Transactions
	var used uint64
	var trans *dataMoverTransactionState
	for i := range transfers {
		t := &transfers[i]
		used += reservedBytes(t)
		if trans == nil && t.NextReadAddr-t.SrcAddress < t.ByteSize {
			trans = t
		}
	}
	if trans == nil || (used >= m.comp.Spec.BufferSize && !m.neededForOldestWrite(trans)) {
		return false
	}
	granularity := trans.Buffer.Granularity
	addr := alignDown(trans.NextReadAddr, granularity)
	srcP := m.portFor(trans.SrcSide)

	req := messaging.Msg{Payload: memprotocol.ReadReq{
		Address: addr,

		AccessByteSize: granularity,
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

	remaining := trans.ByteSize - (trans.NextReadAddr - trans.SrcAddress)
	trans.NextReadAddr += min(granularity-trans.NextReadAddr%granularity, remaining)
	trans.PendingRead[req.ID] = pendingReadState{TaskID: taskID, Offset: addr}

	return true
}

// reservedBytes returns the whole source units issued and not yet released.
func reservedBytes(t *dataMoverTransactionState) uint64 {
	if t.NextReadAddr == t.SrcAddress {
		return 0
	}
	return alignUp(t.NextReadAddr, t.Buffer.Granularity) - t.Buffer.Offset
}

// neededForOldestWrite reports whether trans is the oldest copy that still has
// destination writes to issue and its next read holds a byte of its next write.
func (m *dataTransferMW) neededForOldestWrite(trans *dataMoverTransactionState) bool {
	transfers := m.comp.State.Transactions
	for i := range transfers {
		t := &transfers[i]
		if t.NextWriteAddr-t.DstAddress >= t.ByteSize {
			continue
		}
		if t != trans {
			return false
		}
		srcAddr, count := m.nextWriteSpan(t)
		return t.NextReadAddr-srcAddr < count
	}
	return false
}

// nextWriteSpan returns the source address and byte count of the requested
// bytes that fall in the next destination unit.
func (m *dataTransferMW) nextWriteSpan(t *dataMoverTransactionState) (uint64, uint64) {
	granularity := resolveByteGranularity(m.comp.Spec, t.DstSide)
	written := t.NextWriteAddr - t.DstAddress
	return t.SrcAddress + written, min(granularity-t.NextWriteAddr%granularity, t.ByteSize-written)
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
		if trans.NextWriteAddr-trans.DstAddress >= trans.ByteSize {
			continue
		}
		srcAddr, count := m.nextWriteSpan(trans)
		data, ready := bufferExtractData(&trans.Buffer, srcAddr, count)
		if ready {
			m.sendWrite(dstP, trans, data)
			return true
		}
	}
	return false
}

// fullUnitWrite places payload in the aligned destination unit containing addr.
// A fully covered unit has no mask; otherwise the mask marks exactly payload.
func fullUnitWrite(addr, granularity uint64, payload []byte) memprotocol.WriteReq {
	unit := alignDown(addr, granularity)
	if uint64(len(payload)) == granularity {
		return memprotocol.WriteReq{Address: unit, Data: payload}
	}
	head := addr - unit
	data := make([]byte, granularity)
	copy(data[head:], payload)
	mask := make([]bool, granularity)
	for i := range payload {
		mask[head+uint64(i)] = true
	}
	return memprotocol.WriteReq{Address: unit, Data: data, DirtyMask: mask}
}

func (m *dataTransferMW) sendWrite(dstP messaging.Port, trans *dataMoverTransactionState, payload []byte) {
	granularity := resolveByteGranularity(m.comp.Spec, trans.DstSide)
	write := fullUnitWrite(trans.NextWriteAddr, granularity, payload)
	req := messaging.Msg{Payload: write,
		ID: m.comp.NewID(),

		Src: dstP.AsRemote(),
		Dst: m.mapperFor(trans.DstSide).Find(write.Address),

		TrafficBytes: len(write.Data) + 12,
	}

	taskID := m.comp.TaskTracker().StartReqOut(m.comp, req, trans.RecvTaskID)
	dstP.Send(req)

	trans.NextWriteAddr += uint64(len(payload))
	trans.PendingWrite[req.ID] = taskID
	written := trans.NextWriteAddr - trans.DstAddress
	if written == trans.ByteSize {
		// Release the final source unit even when the copy ends inside it.
		trans.Buffer.Chunks = nil
		trans.Buffer.Offset = alignUp(trans.SrcAddress+written, trans.Buffer.Granularity)
	} else {
		bufferMoveOffsetForwardTo(&trans.Buffer, trans.SrcAddress+written)
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
