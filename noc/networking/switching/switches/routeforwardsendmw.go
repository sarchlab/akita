package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/networking/routing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/tracing"
)

// routeForwardSendMiddleware returns the routeForwardSendMW from the
// component's middleware list (registered at index 0).
func routeForwardSendMiddleware(
	c *modeling.Component[Spec, State, modeling.None],
) *routeForwardSendMW {
	return c.Middlewares()[0].(*routeForwardSendMW)
}

// GetRoutingTable returns the routing table used by the switch. It locates the
// routeForwardSendMW by type rather than by middleware index, so it does not
// depend on the order in which middlewares were registered.
func GetRoutingTable(c *modeling.Component[Spec, State, modeling.None]) routing.Table {
	for _, mw := range c.Middlewares() {
		if rfsMW, ok := mw.(*routeForwardSendMW); ok {
			return rfsMW.routingTable
		}
	}

	panic(fmt.Sprintf("%s: no routeForwardSendMW middleware found", c.Name()))
}

type routeForwardSendMW struct {
	comp         *modeling.Component[Spec, State, modeling.None]
	portIndex    map[messaging.RemotePort]int // remotePort → index in State.PortComplexes
	routingTable routing.Table
}

// ports returns the switch's local ports, in index order aligned with
// State.PortComplexes.
func (m *routeForwardSendMW) ports() []messaging.Port {
	return m.comp.PortsInGroup("Port")
}

// Tick runs sendOut → forward → route.
func (m *routeForwardSendMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendOut() || madeProgress
	madeProgress = m.forward() || madeProgress
	madeProgress = m.route() || madeProgress

	return madeProgress
}

func (m *routeForwardSendMW) route() (madeProgress bool) {
	state := &m.comp.State

	for i := range m.ports() {
		pcs := &state.PortComplexes[i]

		for j := 0; j < pcs.NumInputChannel; j++ {
			if pcs.RouteBuffer.Size() == 0 {
				break
			}

			if !pcs.ForwardBuffer.CanPush() {
				break
			}

			item := pcs.RouteBuffer.Pop()
			outputBufIdx := m.resolveOutputBufIdx(item.RouteTo)
			item.OutputBufIdx = outputBufIdx

			pcs.ForwardBuffer.PushTyped(item)

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *routeForwardSendMW) resolveOutputBufIdx(msgDst messaging.RemotePort) int {
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
	state := &m.comp.State
	occupiedOutputPort := make([]bool, len(m.ports()))

	for offset := 0; offset < len(m.ports()); offset++ {
		i := (state.NextArbPort + offset) % len(m.ports())
		pcs := &state.PortComplexes[i]

		for pcs.ForwardBuffer.Size() > 0 {
			item := pcs.ForwardBuffer.Peek()
			outIdx := item.OutputBufIdx

			if occupiedOutputPort[outIdx] {
				break
			}

			sendBuf := &state.PortComplexes[outIdx].SendOutBuffer
			if !sendBuf.CanPush() {
				break
			}

			pcs.ForwardBuffer.Pop()
			sendBuf.PushTyped(item.Flit)

			tracing.EndTask(m.comp, tracing.TaskEnd{ID: item.TaskID})

			occupiedOutputPort[outIdx] = true
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

			flit := pcs.SendOutBuffer.Peek()
			flit.Src = port.AsRemote()
			flit.Dst = pcs.RemotePort

			port.Send(flit)
			pcs.SendOutBuffer.Pop()
			madeProgress = true
		}
	}

	return madeProgress
}
