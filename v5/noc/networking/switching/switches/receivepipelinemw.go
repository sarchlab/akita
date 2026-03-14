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

func (m *receivePipelineMW) flitParentTaskID(flit *messaging.Flit) string {
	return flit.ID + "_e2e"
}

func (m *receivePipelineMW) flitTaskID(flit *messaging.Flit) string {
	return flit.ID + "_" + m.comp.Name()
}

func (m *receivePipelineMW) startProcessing() (madeProgress bool) {
	cur := m.comp.GetState()

	for i, port := range m.ports {
		curPcs := cur.PortComplexes[i]

		for j := 0; j < curPcs.NumInputChannel; j++ {
			itemI := port.PeekIncoming()
			if itemI == nil {
				break
			}

			next := m.comp.GetNextState()
			nextPcs := &next.PortComplexes[i]

			flit := itemI.(*messaging.Flit)
			item := routedFlit{
				Flit:    *flit,
				TaskID:  m.flitTaskID(flit),
				RouteTo: flit.Msg.Dst,
			}

			if nextPcs.Latency == 0 {
				if !nextPcs.RouteBuffer.CanPush() {
					break
				}
				nextPcs.RouteBuffer.PushTyped(item)
			} else {
				if !nextPcs.Pipeline.CanAccept() {
					break
				}
				nextPcs.Pipeline.Accept(item)
			}

			port.RetrieveIncoming()

			madeProgress = true

			tracing.StartTask(
				m.flitTaskID(flit),
				m.flitParentTaskID(flit),
				m.comp, "flit", "flit_inside_sw",
				flit,
			)
		}
	}

	return madeProgress
}

func (m *receivePipelineMW) movePipeline() (madeProgress bool) {
	next := m.comp.GetNextState()

	for i := range m.ports {
		pcs := &next.PortComplexes[i]
		if pcs.Latency == 0 {
			continue
		}
		madeProgress = pcs.Pipeline.Tick(&pcs.RouteBuffer) || madeProgress
	}

	return madeProgress
}
