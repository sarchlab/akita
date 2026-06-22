package endpoint

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"

	"github.com/sarchlab/akita/v5/tracing"

	// incomingMW handles the network→device path:
	// tryDeliver, assemble, recv.
	"github.com/sarchlab/akita/v5/messaging"
)

type incomingMW struct {
	comp        *modeling.Component[Spec, State, modeling.None]
	devicePorts []messaging.Port
}

// networkPort resolves the endpoint's network port by name. The instance is
// assigned externally after Build, so it is resolved lazily.
func (m *incomingMW) networkPort() messaging.Port {
	return m.comp.GetPortByName("NetworkPort")
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
	spec := m.comp.Spec()
	state := &m.comp.State

	for i := 0; i < spec.NumInputChannels; i++ {
		receivedI := m.networkPort().PeekIncoming()
		if receivedI == nil {
			return madeProgress
		}

		flit := receivedI.(packetization.Flit)
		meta := &flit.Msg

		var assemblingIdx int = -1
		for j, a := range state.AssemblingMsgs {
			if a.MsgID == meta.ID {
				assemblingIdx = j
				break
			}
		}

		if assemblingIdx < 0 {
			state.AssemblingMsgs = append(state.AssemblingMsgs, assemblingMsgState{
				MsgID:           meta.ID,
				NumFlitRequired: flit.NumFlitInMsg,
				NumFlitArrived:  1,
				Payload:         msgHolder{Msg: flit.Payload},
			})
		} else {
			a := &state.AssemblingMsgs[assemblingIdx]
			a.NumFlitArrived++
			// The concrete message rides on a single flit; capture it whenever
			// that flit is the one arriving, regardless of flit order.
			if flit.Payload != nil {
				a.Payload = msgHolder{Msg: flit.Payload}
			}
		}

		m.networkPort().RetrieveIncoming()

		m.logFlitE2ETaskFromFlit(flit, true)

		madeProgress = true
	}

	return madeProgress
}

func (m *incomingMW) assemble() bool {
	madeProgress := false
	state := &m.comp.State

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

		if a.Payload.Msg == nil {
			panic(fmt.Sprintf(
				"message %d reassembled without a payload-bearing flit", a.MsgID))
		}

		state.AssembledMsgs = append(state.AssembledMsgs, a.Payload)
		madeProgress = true
	}

	state.AssemblingMsgs = state.AssemblingMsgs[:writeIdx]

	return madeProgress
}

func (m *incomingMW) tryDeliver() bool {
	madeProgress := false
	state := &m.comp.State

	numDelivered := 0

	for i := 0; i < len(state.AssembledMsgs); i++ {
		msg := state.AssembledMsgs[i].Msg
		dst := msg.Meta().Dst

		var dstPort messaging.Port

		for _, port := range m.devicePorts {
			if port.AsRemote() == dst {
				dstPort = port
				break
			}
		}

		if dstPort == nil {
			panic(fmt.Sprintf("no dst port found for %s", dst))
		}

		if !dstPort.CanDeliver() {
			break
		}

		dstPort.Deliver(msg)

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
	flit packetization.Flit, isEnd bool,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: flit.MsgMeta.ID})
		return
	}

	tracing.StartTask(m.comp, tracing.TaskStart{
		ID:       flit.MsgMeta.ID,
		ParentID: flit.Msg.ID,
		Kind:     "flit_e2e",
		What:     "flit_e2e",
		Location: m.comp.Name() + ".FlitBuf",
		Detail:   flit,
	})
}

func (m *incomingMW) logMsgE2ETask(msg messaging.Msg, isEnd bool) {
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

func (m *incomingMW) logMsgReq(isEnd bool, msg messaging.Msg) {
	taskID := tracing.MsgIDAtReceiver(msg, m.comp)
	if isEnd {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: taskID})
		tracing.ForgetMsgIDAtReceiver(msg.Meta().ID, m.comp)
	} else {
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:       taskID,
			ParentID: msg.Meta().ID,
			Kind:     "msg_e2e",
			What:     "msg_e2e",
			Detail:   msg,
		})
	}
}

func (m *incomingMW) logMsgRsp(isEnd bool, msg messaging.Msg) {
	taskID := tracing.MsgIDAtReceiver(msg, m.comp)
	if isEnd {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: taskID})
		tracing.ForgetMsgIDAtReceiver(msg.Meta().ID, m.comp)
	} else {
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:       taskID,
			ParentID: msg.Meta().ID,
			Kind:     "msg_e2e",
			What:     "msg_e2e",
			Detail:   msg,
		})
	}
}
