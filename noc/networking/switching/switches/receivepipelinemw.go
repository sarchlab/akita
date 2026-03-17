package switches

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type receivePipelineMW struct {
	comp      *modeling.Component[Spec, State]
	ports     []sim.Port
	portIndex map[sim.RemotePort]int
}

// Tick runs movePipeline → startProcessing.
func (m *receivePipelineMW) Tick() bool {
	madeProgress := false

	madeProgress = m.movePipeline() || madeProgress
	madeProgress = m.startProcessing() || madeProgress

	return madeProgress
}

func (m *receivePipelineMW) flitParentTaskID(flit *messaging.Flit) uint64 {
	return flit.MsgMeta.SendTaskID
}

func (m *receivePipelineMW) startProcessing() (madeProgress bool) {
	state := m.comp.GetNextState()

	for i, port := range m.ports {
		pcs := &state.PortComplexes[i]

		for j := 0; j < pcs.NumInputChannel; j++ {
			itemI := port.PeekIncoming()
			if itemI == nil {
				break
			}

			flit := itemI.(*messaging.Flit)
			taskID := sim.GetIDGenerator().Generate()
			item := routedFlit{
				Flit:    *flit,
				TaskID:  taskID,
				RouteTo: flit.Msg.Dst,
			}

			if pcs.Latency == 0 {
				if !pcs.RouteBuffer.CanPush() {
					break
				}
				pcs.RouteBuffer.PushTyped(item)
			} else {
				if !pcs.Pipeline.CanAccept() {
					break
				}
				pcs.Pipeline.Accept(item)
			}

			port.RetrieveIncoming()

			madeProgress = true

			tracing.StartTask(
				taskID,
				m.flitParentTaskID(flit),
				m.comp, "flit", "flit_inside_sw",
				flit,
			)
		}
	}

	return madeProgress
}

func (m *receivePipelineMW) movePipeline() (madeProgress bool) {
	state := m.comp.GetNextState()

	for i := range m.ports {
		pcs := &state.PortComplexes[i]
		if pcs.Latency == 0 {
			continue
		}
		madeProgress = pcs.Pipeline.Tick(&pcs.RouteBuffer) || madeProgress
	}

	return madeProgress
}
