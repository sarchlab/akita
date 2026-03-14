package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// routeForwardSendMiddleware returns the routeForwardSendMW from the
// component's middleware list (registered at index 0).
func routeForwardSendMiddleware(
	c *modeling.Component[Spec, State],
) *routeForwardSendMW {
	return c.Middlewares()[0].(*routeForwardSendMW)
}

// GetRoutingTable returns the routing table used by the switch.
func GetRoutingTable(c *modeling.Component[Spec, State]) routing.Table {
	return routeForwardSendMiddleware(c).routingTable
}

type routeForwardSendMW struct {
	comp         *modeling.Component[Spec, State]
	ports        []sim.Port
	portIndex    map[sim.RemotePort]int // remotePort → index in State.PortComplexes
	routingTable routing.Table
	nextArbPort  int
}

// Tick runs sendOut → forward → route.
func (m *routeForwardSendMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendOut() || madeProgress
	madeProgress = m.forward() || madeProgress
	madeProgress = m.route() || madeProgress

	return madeProgress
}

func (m *routeForwardSendMW) flitTaskID(flit *messaging.Flit) string {
	return flit.ID + "_" + m.comp.Name()
}

func (m *routeForwardSendMW) route() (madeProgress bool) {
	next := m.comp.GetNextState()

	for i := range m.ports {
		pcs := &next.PortComplexes[i]

		for j := 0; j < pcs.NumInputChannel; j++ {
			if pcs.RouteBuffer.Size() == 0 {
				break
			}

			if !pcs.ForwardBuffer.CanPush() {
				break
			}

			item := pcs.RouteBuffer.Elements[0]
			pcs.RouteBuffer.Elements = pcs.RouteBuffer.Elements[1:]
			outputBufIdx := m.resolveOutputBufIdx(item.RouteTo)
			item.Flit.OutputBufIdx = outputBufIdx

			pcs.ForwardBuffer.PushTyped(item)

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *routeForwardSendMW) resolveOutputBufIdx(msgDst sim.RemotePort) int {
	outPort := m.routingTable.FindPort(msgDst)
	if outPort == "" {
		panic(fmt.Sprintf("%s: no output port for %s",
			m.comp.Name(), msgDst))
	}

	idx, ok := m.portIndex[outPort]
	if !ok {
		panic(fmt.Sprintf("%s: no port index for %s",
			m.comp.Name(), outPort))
	}

	return idx
}

func (m *routeForwardSendMW) forward() (madeProgress bool) {
	next := m.comp.GetNextState()
	occupiedOutputPort := make([]bool, len(m.ports))

	for offset := 0; offset < len(m.ports); offset++ {
		i := (m.nextArbPort + offset) % len(m.ports)
		pcs := &next.PortComplexes[i]

		for pcs.ForwardBuffer.Size() > 0 {
			item := pcs.ForwardBuffer.Elements[0]
			outIdx := item.Flit.OutputBufIdx

			if occupiedOutputPort[outIdx] {
				break
			}

			sendBuf := &next.PortComplexes[outIdx].SendOutBuffer
			if !sendBuf.CanPush() {
				break
			}

			pcs.ForwardBuffer.Elements = pcs.ForwardBuffer.Elements[1:]
			sendBuf.PushTyped(item.Flit)
			occupiedOutputPort[outIdx] = true
			madeProgress = true
		}
	}

	m.nextArbPort = (m.nextArbPort + 1) % len(m.ports)

	return madeProgress
}

func (m *routeForwardSendMW) sendOut() (madeProgress bool) {
	cur := m.comp.GetState()

	for i, port := range m.ports {
		curPcs := &cur.PortComplexes[i]
		numSent := 0

		for j := 0; j < curPcs.NumOutputChannel; j++ {
			if numSent >= curPcs.SendOutBuffer.Size() {
				break
			}

			flit := curPcs.SendOutBuffer.Elements[numSent]
			flit.Src = port.AsRemote()
			flit.Dst = curPcs.RemotePort

			err := port.Send(&flit)
			if err == nil {
				madeProgress = true
				numSent++

				tracing.EndTask(m.flitTaskID(&flit), m.comp)
			}
		}

		if numSent > 0 {
			next := m.comp.GetNextState()
			nextPcs := &next.PortComplexes[i]
			nextPcs.SendOutBuffer.Elements =
				nextPcs.SendOutBuffer.Elements[numSent:]
		}
	}

	return madeProgress
}
