package switches

import (
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

type routeForwardSendMW struct {
	comp *Comp
}

// ports returns the switch's local ports, in index order aligned with
// State.PortComplexes.
func (m *routeForwardSendMW) ports() []messaging.Port {
	return m.comp.Ports.Links
}

// Handle runs sendOut → forward → route on every tick.
func (m *routeForwardSendMW) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.sendOut() || madeProgress
	madeProgress = m.forward() || madeProgress
	madeProgress = m.route() || madeProgress

	return madeProgress
}

// route completes one cycle of route processing in place. The immutable
// output index was resolved at admission. Only this prefix can arbitrate
// on the next tick, even when the rest of the shared buffer is occupied.
func (m *routeForwardSendMW) route() (madeProgress bool) {
	for i := range m.comp.State.PortComplexes {
		port := &m.comp.State.PortComplexes[i]
		for ; port.Ready < port.Waiting.Size(); port.Ready++ {
			if tracker := m.comp.TaskTracker(); tracker != nil {
				item, _ := port.Waiting.PeekAt(port.Ready)
				tracker.AddMilestone(tasks.Milestone{
					TaskID: item.TaskID, Kind: tasks.MilestoneKindQueue,
					What: m.comp.Name() + ".Route",
				})
			}
			madeProgress = true
		}
	}
	return madeProgress
}

func (m *routeForwardSendMW) forward() (madeProgress bool) {
	state := &m.comp.State
	sentToOutput := make([]int, len(m.ports()))

	for offset := 0; offset < len(m.ports()); offset++ {
		i := (state.NextArbPort + offset) % len(m.ports())
		pcs := &state.PortComplexes[i]

		// Visit flits oldest first. A blocked output stays blocked throughout
		// this pass, so skipping it preserves each virtual output queue's FIFO.
		for idx, sent := 0, 0; idx < pcs.Ready && sent < pcs.NumInputChannel; {
			item, _ := pcs.Waiting.PeekAt(idx)
			outIdx := item.OutputBufIdx
			out := &state.PortComplexes[outIdx]
			sendBuf := &out.SendOutBuffer
			if sentToOutput[outIdx] >= out.NumOutputChannel || !sendBuf.CanPush() {
				idx++
				continue
			}

			pcs.Waiting.PopAt(idx)
			pcs.Ready--
			sent++
			// Push the whole routedFlit (carrying its TaskID) so the in-switch
			// "flit" task can be ended in sendOut, once the flit actually
			// leaves on the output link, rather than here.
			sendBuf.Push(item)

			// The flit waited in its virtual output queue for the output port to win
			// arbitration and for a send-buffer slot downstream.
			if m.comp.TaskTracker() != nil {
				m.comp.TaskTracker().AddMilestone(tasks.Milestone{
					TaskID: item.TaskID,
					Kind:   tasks.MilestoneKindNetworkBusy,
					What:   m.comp.Name() + ".Waiting",
				})
			}

			sentToOutput[outIdx]++
			madeProgress = true
		}
	}

	state.NextArbPort = (state.NextArbPort + 1) % len(m.ports())

	return madeProgress
}

func (m *routeForwardSendMW) sendOut() (madeProgress bool) {
	state := &m.comp.State

	for i, port := range m.ports() {
		pcs := &state.PortComplexes[i]

		for j := 0; j < pcs.NumOutputChannel; j++ {
			if pcs.SendOutBuffer.Size() == 0 {
				break
			}

			if !port.CanSend() {
				break
			}

			item, _ := pcs.SendOutBuffer.Peek()
			flit := item.Flit
			flit.Src = port.AsRemote()
			flit.Dst = pcs.RemotePort

			port.Send(flit)
			pcs.SendOutBuffer.Pop()

			// The flit waited in the send-out buffer for the output link to be
			// free; this is the last milestone before the task ends.
			if m.comp.TaskTracker() != nil {
				m.comp.TaskTracker().AddMilestone(tasks.Milestone{
					TaskID: item.TaskID,
					Kind:   tasks.MilestoneKindNetworkBusy,
					What:   port.Name(),
				})
			}

			// The flit has now left the switch; close the in-switch "flit"
			// task that opened when it entered the receive pipeline.
			m.comp.TaskTracker().EndTask(item.TaskID)

			madeProgress = true
		}
	}

	return madeProgress
}
