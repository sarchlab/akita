package switches

import (
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type receivePipelineMW struct {
	comp      *Comp
	portIndex map[messaging.RemotePort]int
}

// ports returns the switch's local ports, in index order aligned with
// State.PortComplexes.
func (m *receivePipelineMW) ports() []messaging.Port {
	return m.comp.Ports.Port
}

// Handle runs movePipeline → startProcessing on every tick.
func (m *receivePipelineMW) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.movePipeline() || madeProgress
	madeProgress = m.startProcessing() || madeProgress

	return madeProgress
}

func (m *receivePipelineMW) flitParentTaskID(flit messaging.Msg) uint64 {
	return flit.ID
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

			flit := itemI
			taskID := m.comp.NewID()
			item := routedFlit{
				Flit:    flit,
				TaskID:  taskID,
				RouteTo: flit.Payload.(packetization.Flit).Msg.Dst,
			}

			if pcs.Latency == 0 {
				if !pcs.RouteBuffer.CanPush() {
					break
				}
				pcs.RouteBuffer.Push(item)
			} else {
				if !pcs.Pipeline.CanAccept() {
					break
				}
				pcs.Pipeline.Accept(item)
			}

			port.RetrieveIncoming()

			madeProgress = true

			tracing.StartTask(m.comp, tracing.TaskStart{
				ID:       taskID,
				ParentID: m.flitParentTaskID(flit),
				Kind:     "flit",
				What:     "flit_inside_sw",
				Detail:   flit,
			})
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
		madeProgress = pcs.Pipeline.Tick(&pcs.RouteBuffer) || madeProgress
	}

	return madeProgress
}
