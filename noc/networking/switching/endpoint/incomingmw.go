package endpoint

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// incomingMW handles the network→device path:
// tryDeliver, assemble, recv.
type incomingMW struct {
	comp        *modeling.Component[Spec, State]
	devicePorts []sim.Port
	networkPort sim.Port
}

// Tick runs the incoming stages.
func (m *incomingMW) Tick() bool {
	madeProgress := false

	madeProgress = m.tryDeliver() || madeProgress
	madeProgress = m.assemble() || madeProgress
	madeProgress = m.recv() || madeProgress

	return madeProgress
}

func (m *incomingMW) recv() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()

	for i := 0; i < spec.NumInputChannels; i++ {
		receivedI := m.networkPort.PeekIncoming()
		if receivedI == nil {
			return madeProgress
		}

		flit := receivedI.(*messaging.Flit)
		msg := &flit.Msg

		var assemblingIdx int = -1
		for j, a := range state.AssemblingMsgs {
			if a.MsgID == msg.ID {
				assemblingIdx = j
				break
			}
		}

		if assemblingIdx < 0 {
			state.AssemblingMsgs = append(state.AssemblingMsgs, assemblingMsgState{
				MsgID:           msg.ID,
				Src:             msg.Src,
				Dst:             msg.Dst,
				RspTo:           msg.RspTo,
				TrafficClass:    msg.TrafficClass,
				TrafficBytes:    msg.TrafficBytes,
				NumFlitRequired: flit.NumFlitInMsg,
				NumFlitArrived:  1,
			})
		} else {
			state.AssemblingMsgs[assemblingIdx].NumFlitArrived++
		}

		m.networkPort.RetrieveIncoming()

		m.logFlitE2ETaskFromFlit(flit, true)

		madeProgress = true
	}

	return madeProgress
}

func (m *incomingMW) assemble() bool {
	madeProgress := false
	state := m.comp.GetNextState()

	if len(state.AssemblingMsgs) == 0 {
		return false
	}

	// Compact in-place: move incomplete entries to the front.
	writeIdx := 0

	for i := range state.AssemblingMsgs {
		a := &state.AssemblingMsgs[i]
		if a.NumFlitArrived < a.NumFlitRequired {
			if writeIdx != i {
				state.AssemblingMsgs[writeIdx] = *a
			}
			writeIdx++
			continue
		}

		assembled := sim.MsgMeta{
			ID:           a.MsgID,
			Src:          a.Src,
			Dst:          a.Dst,
			RspTo:        a.RspTo,
			TrafficClass: a.TrafficClass,
			TrafficBytes: a.TrafficBytes,
		}
		state.AssembledMsgs = append(state.AssembledMsgs, assembled)
		madeProgress = true
	}

	state.AssemblingMsgs = state.AssemblingMsgs[:writeIdx]

	return madeProgress
}

func (m *incomingMW) tryDeliver() bool {
	madeProgress := false
	state := m.comp.GetNextState()

	numDelivered := 0

	for i := 0; i < len(state.AssembledMsgs); i++ {
		meta := state.AssembledMsgs[i]
		dst := meta.Dst

		var dstPort sim.Port

		for _, port := range m.devicePorts {
			if port.AsRemote() == dst {
				dstPort = port
				break
			}
		}

		if dstPort == nil {
			panic(fmt.Sprintf("no dst port found for %s", dst))
		}

		msg := &sim.MsgMeta{
			ID:           meta.ID,
			Src:          meta.Src,
			Dst:          meta.Dst,
			RspTo:        meta.RspTo,
			TrafficClass: meta.TrafficClass,
			TrafficBytes: meta.TrafficBytes,
		}

		err := dstPort.Deliver(msg)
		if err != nil {
			break
		}

		m.logMsgE2ETask(msg, true)

		numDelivered++
		madeProgress = true
	}

	if numDelivered > 0 {
		state.AssembledMsgs = state.AssembledMsgs[numDelivered:]
	}

	return madeProgress
}

func (m *incomingMW) logFlitE2ETaskFromFlit(
	flit *messaging.Flit, isEnd bool,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(flit.MsgMeta.SendTaskID, m.comp)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		flit.MsgMeta.SendTaskID, flit.Msg.SendTaskID,
		m.comp, "flit_e2e", "flit_e2e", m.comp.Name()+".FlitBuf", flit,
	)
}

func (m *incomingMW) logMsgE2ETask(msg sim.Msg, isEnd bool) {
	if m.comp.NumHooks() == 0 {
		return
	}

	meta := msg.Meta()

	if meta.IsRsp() {
		m.logMsgRsp(isEnd, msg)
		return
	}

	m.logMsgReq(isEnd, msg)
}

func (m *incomingMW) logMsgReq(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if meta.RecvTaskID == 0 {
		meta.RecvTaskID = sim.GetIDGenerator().Generate()
	}
	if isEnd {
		tracing.EndTask(meta.RecvTaskID, m.comp)
	} else {
		tracing.StartTask(
			meta.RecvTaskID,
			meta.SendTaskID,
			m.comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *incomingMW) logMsgRsp(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if meta.RecvTaskID == 0 {
		meta.RecvTaskID = sim.GetIDGenerator().Generate()
	}
	if isEnd {
		tracing.EndTask(meta.RecvTaskID, m.comp)
	} else {
		tracing.StartTask(
			meta.RecvTaskID,
			meta.SendTaskID,
			m.comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}
