package idealmemcontroller

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type memMiddleware struct {
	comp *Comp
}

func (m *memMiddleware) topPort() messaging.Port {
	return m.comp.Ports.Top
}

// Handle admits new requests and advances the in-flight accesses by one
// cycle.
func (m *memMiddleware) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.takeNewReqs() || madeProgress
	madeProgress = m.processCountdowns() || madeProgress

	return madeProgress
}

func (m *memMiddleware) takeNewReqs() (madeProgress bool) {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}

	spec := m.comp.Spec

	for i := 0; i < spec.Width; i++ {
		msgI, ok := m.topPort().RetrieveIncoming()
		if !ok {
			break
		}

		msg := msgI
		tracing.TraceReqReceive(m.comp, msg)

		tx := m.msgToInflightTransaction(msg)

		state.InflightTransactions = append(
			state.InflightTransactions, tx)
		madeProgress = true
	}

	return madeProgress
}

func (m *memMiddleware) msgToInflightTransaction(msg messaging.Msg) inflightTransaction {
	spec := m.comp.Spec
	recvTaskID := tracing.MsgIDAtReceiver(msg, m.comp)

	switch content := msg.Payload.(type) {
	case memprotocol.ReadReq:
		payload := msg

		return inflightTransaction{
			CycleLeft:      spec.Latency,
			Address:        content.Address,
			AccessByteSize: content.AccessByteSize,
			ReqID:          payload.ID,
			RecvTaskID:     recvTaskID,
			IsRead:         true,
			Src:            payload.Src,
		}
	case memprotocol.WriteReq:
		payload := msg

		return inflightTransaction{
			CycleLeft:      spec.Latency,
			Address:        content.Address,
			AccessByteSize: uint64(len(content.Data)),
			ReqID:          payload.ID,
			RecvTaskID:     recvTaskID,
			IsRead:         false,
			Data:           content.Data,
			DirtyMask:      content.DirtyMask,
			Src:            payload.Src,
		}
	default:
		log.Panicf("cannot handle request of type %T", msg.Payload)
		return inflightTransaction{}
	}
}

func (m *memMiddleware) processCountdowns() bool {
	state := &m.comp.State
	if state.ControlState == memcontrolprotocol.StatePaused {
		return false
	}

	madeProgress := false
	remaining := make([]inflightTransaction, 0, len(state.InflightTransactions))

	for i := range state.InflightTransactions {
		tx := state.InflightTransactions[i]

		if tx.CycleLeft > 0 {
			tx.CycleLeft--
			madeProgress = true
		}

		if tx.CycleLeft == 0 {
			sent := m.sendResponse(&tx)
			if sent {
				madeProgress = true
				continue // remove from list
			}
		}

		remaining = append(remaining, tx)
	}

	state.InflightTransactions = remaining

	return madeProgress
}

func (m *memMiddleware) sendResponse(tx *inflightTransaction) bool {
	if tx.IsRead {
		return m.sendReadResponse(tx)
	}

	return m.sendWriteResponse(tx)
}

func (m *memMiddleware) sendReadResponse(tx *inflightTransaction) bool {
	data := m.comp.Resources.Storage.Read(tx.Address, tx.AccessByteSize)

	rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{
		Data: data},
		ID:    m.comp.NewID(),
		Src:   m.topPort().AsRemote(),
		Dst:   tx.Src,
		RspTo: tx.ReqID,

		TrafficBytes: len(data) + 4,
		TrafficClass: "memprotocol.DataReadyRsp"}

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	m.traceReqComplete(tx.RecvTaskID, tx.ReqID)

	return true
}

func (m *memMiddleware) sendWriteResponse(tx *inflightTransaction) bool {
	rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          tx.Src,
		RspTo:        tx.ReqID,
		TrafficBytes: 4,
		TrafficClass: "memprotocol.WriteDoneRsp"}

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	addr := tx.Address

	if tx.DirtyMask == nil {
		m.comp.Resources.Storage.Write(addr, tx.Data)
	} else {
		data := m.comp.Resources.Storage.Read(addr, uint64(len(tx.Data)))

		for i := 0; i < len(tx.Data); i++ {
			if tx.DirtyMask[i] {
				data[i] = tx.Data[i]
			}
		}

		m.comp.Resources.Storage.Write(addr, data)
	}

	m.traceReqComplete(tx.RecvTaskID, tx.ReqID)

	return true
}

func (m *memMiddleware) traceReqComplete(recvTaskID, reqMsgID uint64) {
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: recvTaskID})
	tracing.ForgetMsgIDAtReceiver(reqMsgID, m.comp)
}
