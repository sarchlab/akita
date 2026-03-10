package idealmemcontroller

import (
	"fmt"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type memMiddleware struct {
	*Comp
}

func (m *memMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.takeNewReqs() || madeProgress
	madeProgress = m.processCountdowns() || madeProgress

	return madeProgress
}

func (m *memMiddleware) takeNewReqs() (madeProgress bool) {
	state := m.Component.GetState()
	if state.CurrentState != "enable" {
		return false
	}

	spec := m.Component.GetSpec()

	for i := 0; i < spec.Width; i++ {
		msgI := m.topPort.RetrieveIncoming()
		if msgI == nil {
			break
		}

		msg := msgI.(*sim.GenericMsg)
		tracing.TraceReqReceive(msg, m)

		tx := m.msgToInflightTransaction(msg)
		state.InflightTransactions = append(state.InflightTransactions, tx)
		madeProgress = true
	}

	m.Component.SetState(state)

	return madeProgress
}

func (m *memMiddleware) msgToInflightTransaction(msg interface{}) inflightTransaction {
	spec := m.Component.GetSpec()
	simMsg := msg.(*sim.GenericMsg)

	switch payload := simMsg.Payload.(type) {
	case *mem.ReadReqPayload:
		return inflightTransaction{
			CycleLeft:      spec.Latency,
			Address:        payload.Address,
			AccessByteSize: payload.AccessByteSize,
			ReqID:          simMsg.ID,
			IsRead:         true,
			Src:            simMsg.Src,
		}
	case *mem.WriteReqPayload:
		return inflightTransaction{
			CycleLeft:      spec.Latency,
			Address:        payload.Address,
			AccessByteSize: uint64(len(payload.Data)),
			ReqID:          simMsg.ID,
			IsRead:         false,
			Data:           payload.Data,
			DirtyMask:      payload.DirtyMask,
			Src:            simMsg.Src,
		}
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(simMsg.Payload))
		return inflightTransaction{}
	}
}

func (m *memMiddleware) processCountdowns() bool {
	state := m.Component.GetState()
	if state.CurrentState == "pause" {
		return false
	}

	madeProgress := false
	remaining := make([]inflightTransaction, 0, len(state.InflightTransactions))

	for i := range state.InflightTransactions {
		tx := &state.InflightTransactions[i]

		if tx.CycleLeft > 0 {
			tx.CycleLeft--
			madeProgress = true
		}

		if tx.CycleLeft == 0 {
			sent := m.sendResponse(tx)
			if sent {
				madeProgress = true
				continue // remove from list
			}
		}

		remaining = append(remaining, *tx)
	}

	state.InflightTransactions = remaining
	m.Component.SetState(state)

	return madeProgress
}

func (m *memMiddleware) sendResponse(tx *inflightTransaction) bool {
	if tx.IsRead {
		return m.sendReadResponse(tx)
	}

	return m.sendWriteResponse(tx)
}

func (m *memMiddleware) sendReadResponse(tx *inflightTransaction) bool {
	addr := tx.Address
	if m.addressConverter != nil {
		addr = m.addressConverter.ConvertExternalToInternal(addr)
	}

	data, err := m.Storage.Read(addr, tx.AccessByteSize)
	if err != nil {
		log.Panic(err)
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(tx.Src).
		WithRspTo(tx.ReqID).
		WithData(data).
		Build()

	networkErr := m.topPort.Send(rsp)
	if networkErr != nil {
		return false
	}

	m.traceReqComplete(tx.ReqID)

	return true
}

func (m *memMiddleware) sendWriteResponse(tx *inflightTransaction) bool {
	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(tx.Src).
		WithRspTo(tx.ReqID).
		Build()

	networkErr := m.topPort.Send(rsp)
	if networkErr != nil {
		return false
	}

	addr := tx.Address
	if m.addressConverter != nil {
		addr = m.addressConverter.ConvertExternalToInternal(addr)
	}

	if tx.DirtyMask == nil {
		err := m.Storage.Write(addr, tx.Data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		data, err := m.Storage.Read(addr, uint64(len(tx.Data)))
		if err != nil {
			panic(err)
		}

		for i := 0; i < len(tx.Data); i++ {
			if tx.DirtyMask[i] {
				data[i] = tx.Data[i]
			}
		}

		err = m.Storage.Write(addr, data)
		if err != nil {
			panic(err)
		}
	}

	m.traceReqComplete(tx.ReqID)

	return true
}

func (m *memMiddleware) traceReqComplete(reqID string) {
	taskID := fmt.Sprintf("%s@%s", reqID, m.Name())
	tracing.EndTask(taskID, m)
}
