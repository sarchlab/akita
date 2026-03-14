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

func (m *incomingMW) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (m *incomingMW) flitTaskID(flitID string) string {
	return fmt.Sprintf("%s_e2e", flitID)
}

func (m *incomingMW) recv() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	for i := 0; i < spec.NumInputChannels; i++ {
		receivedI := m.networkPort.PeekIncoming()
		if receivedI == nil {
			return madeProgress
		}

		flit := receivedI.(*messaging.Flit)
		msg := &flit.Msg

		var assemblingIdx int = -1
		for j, a := range next.AssemblingMsgs {
			if a.MsgID == msg.ID {
				assemblingIdx = j
				break
			}
		}

		if assemblingIdx < 0 {
			next.AssemblingMsgs = append(next.AssemblingMsgs, assemblingMsgState{
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
			next.AssemblingMsgs[assemblingIdx].NumFlitArrived++
		}

		m.networkPort.RetrieveIncoming()

		m.logFlitE2ETaskFromFlit(flit, true)

		madeProgress = true
	}

	return madeProgress
}

func (m *incomingMW) assemble() bool {
	madeProgress := false
	cur := m.comp.GetState()

	if len(cur.AssemblingMsgs) == 0 {
		return false
	}

	next := m.comp.GetNextState()

	// Compact in-place: move incomplete entries to the front.
	writeIdx := 0

	for i := range next.AssemblingMsgs {
		a := &next.AssemblingMsgs[i]
		if a.NumFlitArrived < a.NumFlitRequired {
			if writeIdx != i {
				next.AssemblingMsgs[writeIdx] = *a
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
		next.AssembledMsgs = append(next.AssembledMsgs, assembled)
		madeProgress = true
	}

	next.AssemblingMsgs = next.AssemblingMsgs[:writeIdx]

	return madeProgress
}

func (m *incomingMW) tryDeliver() bool {
	madeProgress := false
	cur := m.comp.GetState()

	numDelivered := 0

	for i := 0; i < len(cur.AssembledMsgs); i++ {
		meta := cur.AssembledMsgs[i]
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
		next := m.comp.GetNextState()
		next.AssembledMsgs = next.AssembledMsgs[numDelivered:]
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
		tracing.EndTask(m.flitTaskID(flit.ID), m.comp)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(flit.ID), m.msgTaskID(flit.Msg.ID),
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
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m.comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.ID+"_req_out",
			m.comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *incomingMW) logMsgRsp(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m.comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.RspTo+"_req_out",
			m.comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}
