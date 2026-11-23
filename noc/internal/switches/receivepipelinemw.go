package switches

import (
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

type receivePipelineMW struct {
	comp      *Comp
	portIndex map[messaging.RemotePort]int
}

// ports returns the switch's local ports, in index order aligned with
// State.PortComplexes.
func (m *receivePipelineMW) ports() []messaging.Port {
	return m.comp.Ports.Links
}

// Handle runs movePipeline → startProcessing on every tick. When the tick makes
// no progress, the switch sleeps until a flit in a pipeline is due.
func (m *receivePipelineMW) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.movePipeline() || madeProgress
	madeProgress = m.startProcessing() || madeProgress

	if !madeProgress {
		m.scheduleWake()
	}

	return madeProgress
}

// scheduleWake asks for a tick at the next cycle a pipeline flit becomes ready.
func (m *receivePipelineMW) scheduleWake() {
	state := &m.comp.State
	wake, found := timing.VTimeInPicoSec(0), false

	for i := range state.PortComplexes {
		due, ok := state.PortComplexes[i].Pipeline.NextReadyTime()
		if ok && (!found || due < wake) {
			wake, found = due, true
		}
	}

	if found {
		m.comp.TickAt(wake)
	}
}

func (m *receivePipelineMW) startProcessing() (madeProgress bool) {
	state := &m.comp.State

	for i, port := range m.ports() {
		pcs := &state.PortComplexes[i]

		for j := 0; j < pcs.NumInputChannel; j++ {
			itemI, ok := port.PeekIncoming()
			if !ok {
				break
			}

			// Start work only when admission commits; blocked retries have no task.
			if pcs.Latency == 0 && !pcs.RouteBuffer.CanPush() {
				break
			}
			if pcs.Latency != 0 && !pcs.Pipeline.CanPush() {
				break
			}

			flit := itemI
			taskID := m.comp.TaskTracker().StartTask(m.comp, tasks.StartInfo{
				ParentID: flit.Payload.(packetization.Flit).FlitTaskID,
				Kind:     "flit",
				What:     "flit_inside_sw",
				Detail:   flit,
			})
			item := routedFlit{
				Flit:    flit,
				TaskID:  taskID,
				RouteTo: flit.Payload.(packetization.Flit).Dst,
			}

			if pcs.Latency == 0 {
				pcs.RouteBuffer.Push(item)
			} else {
				pcs.Pipeline.Push(item)
			}

			port.RetrieveIncoming()

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *receivePipelineMW) movePipeline() (madeProgress bool) {
	state := &m.comp.State

	for i := range m.ports() {
		pcs := &state.PortComplexes[i]
		if pcs.Latency == 0 {
			continue
		}
		madeProgress = pcs.Pipeline.DrainReady(&pcs.RouteBuffer) || madeProgress
	}

	return madeProgress
}
