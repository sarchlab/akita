package endpoint

import (
	"fmt"

	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/tracing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

// incomingMW handles the network→device path: tryDeliver, assemble, recv.
type incomingMW struct {
	comp        *Comp
	devicePorts []messaging.Port
}

// networkPort resolves the endpoint's network port by name. The instance is
// assigned externally after Build, so it is resolved lazily.
func (m *incomingMW) networkPort() messaging.Port {
	return m.comp.Ports.NetworkPort
}

// Handle runs the incoming stages on every tick.
func (m *incomingMW) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.tryDeliver() || madeProgress
	madeProgress = m.assemble() || madeProgress
	madeProgress = m.recv() || madeProgress

	return madeProgress
}

func (m *incomingMW) recv() bool {
	madeProgress := false
	spec := m.comp.Spec
	state := &m.comp.State

	for i := 0; i < spec.NumInputChannels; i++ {
		receivedI, ok := m.networkPort().PeekIncoming()
		if !ok {
			return madeProgress
		}

		flit := receivedI
		part := flit.Payload.(packetization.Flit)
		if part.NumFlitInMsg <= 0 || part.SeqID < 0 || part.SeqID >= part.NumFlitInMsg {
			panic("endpoint: invalid flit sequence")
		}

		var assemblingIdx int = -1
		for j, a := range state.AssemblingMsgs {
			if a.MsgID == part.MsgID {
				assemblingIdx = j
				break
			}
		}

		if assemblingIdx < 0 {
			assemblingIdx = len(state.AssemblingMsgs)
			state.AssemblingMsgs = append(state.AssemblingMsgs, assemblingMsgState{
				MsgID:     part.MsgID,
				MsgTaskID: part.MsgTaskID,
				Received:  make([]bool, part.NumFlitInMsg),
			})
		}
		a := &state.AssemblingMsgs[assemblingIdx]
		if len(a.Received) != part.NumFlitInMsg || a.Received[part.SeqID] {
			panic("endpoint: inconsistent or duplicate flit")
		}
		if part.SeqID == 0 {
			if part.Msg.ID != part.MsgID || part.Msg.Dst != part.Dst {
				panic("endpoint: head flit does not match its message")
			}
			a.Msg = part.Msg
		}
		a.Received[part.SeqID] = true
		a.NumFlitArrived++

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
		if a.NumFlitArrived < len(a.Received) {
			if writeIdx != i {
				state.AssemblingMsgs[writeIdx] = *a
			}
			writeIdx++
			continue
		}

		state.AssembledMsgs = append(state.AssembledMsgs, a.Msg)

		// The message is fully reassembled; close its msg_e2e task (the parent
		// of all of this message's flit_e2e tasks).
		m.logMsgE2EEnd(a.MsgTaskID)

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
		msg := state.AssembledMsgs[i]
		dst := msg.Dst

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

		numDelivered++
		madeProgress = true
	}

	if numDelivered > 0 {
		state.AssembledMsgs = state.AssembledMsgs[numDelivered:]
	}

	return madeProgress
}

func (m *incomingMW) logFlitE2ETaskFromFlit(
	flit messaging.Msg, isEnd bool,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		// The flit just arrived after traversing the network; the span since it
		// was sent is its in-network transfer time (refined by the per-switch
		// flit subtasks). Last milestone before the task ends.
		tracing.AddMilestone(m.comp, tracing.Milestone{
			TaskID: flit.ID,
			Kind:   tracing.MilestoneKindNetworkTransfer,
			What:   m.comp.Name() + ".NetworkPort",
		})
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: flit.ID})
		return
	}

	tracing.StartTask(m.comp, tracing.TaskStart{
		ID:       flit.ID,
		ParentID: flit.Payload.(packetization.Flit).MsgID,
		Kind:     "flit_e2e",
		What:     "flit_e2e",
		Location: m.comp.Name() + ".FlitBuf",
		Detail:   flit,
	})
}

// logMsgE2EEnd closes the per-message msg_e2e task once the message is
// reassembled, charging the whole span since it was sent to network transfer.
// The task was opened by the sending endpoint (logMsgE2EStart) and is keyed by
// the MsgTaskID carried in every flit.
func (m *incomingMW) logMsgE2EEnd(msgTaskID uint64) {
	if m.comp.NumHooks() == 0 {
		return
	}

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: msgTaskID,
		Kind:   tracing.MilestoneKindNetworkTransfer,
		What:   m.comp.Name() + ".NetworkPort",
	})
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: msgTaskID})
}
