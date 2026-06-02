package idealmemcontroller

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type memMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *memMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *memMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.takeNewReqs() || madeProgress
	madeProgress = m.processCountdowns() || madeProgress

	return madeProgress
}

func (m *memMiddleware) takeNewReqs() (madeProgress bool) {
	state := &m.comp.State
	if state.CurrentState != "enable" {
		return false
	}

	spec := m.comp.Spec()

	for i := 0; i < spec.Width; i++ {
		msgI := m.topPort().RetrieveIncoming()
		if msgI == nil {
			break
		}

		msg := msgI.(messaging.Msg)
		tracing.TraceReqReceive(msg, m.comp)

		tx := m.msgToInflightTransaction(msg)

		state.InflightTransactions = append(
			state.InflightTransactions, tx)
		madeProgress = true
	}

	return madeProgress
}

func (m *memMiddleware) msgToInflightTransaction(msg interface{}) inflightTransaction {
	spec := m.comp.Spec()

	switch payload := msg.(type) {
	case *mem.ReadReq:
		return inflightTransaction{
			CycleLeft:      spec.Latency,
			Address:        payload.Address,
			AccessByteSize: payload.AccessByteSize,
			ReqID:          payload.ID,
			RecvTaskID:     payload.RecvTaskID,
			IsRead:         true,
			Src:            payload.Src,
		}
	case *mem.WriteReq:
		return inflightTransaction{
			CycleLeft:      spec.Latency,
			Address:        payload.Address,
			AccessByteSize: uint64(len(payload.Data)),
			ReqID:          payload.ID,
			RecvTaskID:     payload.RecvTaskID,
			IsRead:         false,
			Data:           payload.Data,
			DirtyMask:      payload.DirtyMask,
			Src:            payload.Src,
		}
	default:
		log.Panicf("cannot handle request of type %T", msg)
		return inflightTransaction{}
	}
}

func (m *memMiddleware) processCountdowns() bool {
	state := &m.comp.State
	if state.CurrentState == "pause" {
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
	spec := m.comp.Spec()
	addr := mem.ConvertAddress(
		spec.AddrConvKind, spec.AddrOffset,
		spec.AddrInterleavingSize, spec.AddrTotalNumOfElements,
		spec.AddrCurrentElementIndex, tx.Address,
	)

	data, err := m.comp.Resources().Storage.Read(addr, tx.AccessByteSize)
	if err != nil {
		log.Panic(err)
	}

	rsp := &mem.DataReadyRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = tx.Src
	rsp.RspTo = tx.ReqID
	rsp.Data = data
	rsp.TrafficBytes = len(data) + 4
	rsp.TrafficClass = "mem.DataReadyRsp"

	networkErr := m.topPort().Send(rsp)
	if networkErr != nil {
		return false
	}

	m.traceReqComplete(tx.RecvTaskID)

	return true
}

func (m *memMiddleware) sendWriteResponse(tx *inflightTransaction) bool {
	rsp := &mem.WriteDoneRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = tx.Src
	rsp.RspTo = tx.ReqID
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.WriteDoneRsp"

	networkErr := m.topPort().Send(rsp)
	if networkErr != nil {
		return false
	}

	spec := m.comp.Spec()
	addr := mem.ConvertAddress(
		spec.AddrConvKind, spec.AddrOffset,
		spec.AddrInterleavingSize, spec.AddrTotalNumOfElements,
		spec.AddrCurrentElementIndex, tx.Address,
	)

	if tx.DirtyMask == nil {
		err := m.comp.Resources().Storage.Write(addr, tx.Data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		data, err := m.comp.Resources().Storage.Read(addr, uint64(len(tx.Data)))
		if err != nil {
			panic(err)
		}

		for i := 0; i < len(tx.Data); i++ {
			if tx.DirtyMask[i] {
				data[i] = tx.Data[i]
			}
		}

		err = m.comp.Resources().Storage.Write(addr, data)
		if err != nil {
			panic(err)
		}
	}

	m.traceReqComplete(tx.RecvTaskID)

	return true
}

func (m *memMiddleware) traceReqComplete(recvTaskID uint64) {
	tracing.EndTask(recvTaskID, m.comp)
}
