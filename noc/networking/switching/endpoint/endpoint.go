// Package endpoint provides endpoint
package endpoint

import (
	"container/list"
	"fmt"
	"math"

	"github.com/sarchlab/akita/v4/noc/messaging"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type msgToAssemble struct {
	msg             sim.Msg
	numFlitRequired int
	numFlitArrived  int
}

// Comp is an akita component(Endpoint) that delegates sending and receiving
// actions of a few ports.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	NetworkPort      sim.Port
	DevicePorts      []sim.Port
	DefaultSwitchDst sim.RemotePort

	numInputChannels  int
	numOutputChannels int
	flitByteSize      int
	encodingOverhead  float64
	msgOutBuf         []sim.Msg
	flitsToSend       []*messaging.Flit

	assemblingMsgTable map[string]*list.Element
	assemblingMsgs     *list.List
	assembledMsgs      []sim.Msg
}

// PlugIn connects a port to the endpoint.
func (c *Comp) PlugIn(port sim.Port) {
	port.SetConnection(c)
	c.DevicePorts = append(c.DevicePorts, port)
}

// NotifyAvailable triggers the endpoint to continue to tick.
func (c *Comp) NotifyAvailable(_ sim.Port) {
	c.TickLater()
}

// NotifySend is called by a port to notify the connection there are
// messages waiting to be sent, can start tick
func (c *Comp) NotifySend() {
	c.TickLater()
}

// Unplug removes the association of a port and an endpoint.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick update the endpoint state.
func (m *middleware) Tick() bool {
	m.Comp.Lock()
	defer m.Comp.Unlock()

	madeProgress := false

	madeProgress = m.sendFlitOut() || madeProgress
	madeProgress = m.prepareMsg() || madeProgress
	madeProgress = m.prepareFlits() || madeProgress
	madeProgress = m.tryDeliver() || madeProgress
	madeProgress = m.assemble() || madeProgress
	madeProgress = m.recv() || madeProgress

	return madeProgress
}

func (m *middleware) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (m *middleware) flitTaskID(flit sim.Msg) string {
	return fmt.Sprintf("%s_e2e", flit.Meta().ID)
}

func (m *middleware) sendFlitOut() bool {
	madeProgress := false

	for i := 0; i < m.numOutputChannels; i++ {
		if len(m.flitsToSend) == 0 {
			return madeProgress
		}

		flit := m.flitsToSend[0]
		err := m.NetworkPort.Send(flit)

		if err == nil {
			m.flitsToSend = m.flitsToSend[1:]

			// fmt.Printf("%.10f, %s, ep send flit, %s, %d\n",
			// 	c.Engine.CurrentTime(), c.Name(),
			// 	flit.Meta().ID, len(c.flitsToSend))

			if len(m.flitsToSend) == 0 {
				for _, p := range m.DevicePorts {
					p.NotifyAvailable()
				}
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) prepareMsg() bool {
	madeProgress := false

	for i := 0; i < len(m.DevicePorts); i++ {
		port := m.DevicePorts[i]
		if port.PeekOutgoing() == nil {
			continue
		}

		msg := port.RetrieveOutgoing()
		m.msgOutBuf = append(m.msgOutBuf, msg)

		// fmt.Printf("%.10f, %s, ep send msg, msg-%s\n",
		// 	now, c.Name(), msg.Meta().ID)

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) prepareFlits() bool {
	madeProgress := false

	for {
		if len(m.msgOutBuf) == 0 {
			return madeProgress
		}

		msg := m.msgOutBuf[0]
		m.msgOutBuf = m.msgOutBuf[1:]
		flits := m.msgToFlits(msg)
		m.flitsToSend = append(m.flitsToSend, flits...)

		// fmt.Printf("%.10f, %s, ep create flit, msg-%s, %d, %d\n",
		// 	c.Engine.CurrentTime(), c.Name(), msg.Meta().ID, len(flits),
		// 	len(c.flitsToSend))

		for _, flit := range flits {
			m.logFlitE2ETask(flit, false)
		}

		madeProgress = true
	}
}

func (m *middleware) recv() bool {
	madeProgress := false

	for i := 0; i < m.numInputChannels; i++ {
		received := m.NetworkPort.PeekIncoming()
		if received == nil {
			return madeProgress
		}

		flit := received.(*messaging.Flit)
		msg := flit.Msg

		assemblingElem := m.assemblingMsgTable[msg.Meta().ID]
		if assemblingElem == nil {
			assemblingElem = m.assemblingMsgs.PushBack(&msgToAssemble{
				msg:             msg,
				numFlitRequired: flit.NumFlitInMsg,
				numFlitArrived:  0,
			})
			m.assemblingMsgTable[msg.Meta().ID] = assemblingElem
		}

		assembling := assemblingElem.Value.(*msgToAssemble)
		assembling.numFlitArrived++

		m.NetworkPort.RetrieveIncoming()

		// fmt.Printf("%.10f, %s, ep received flit %s\n",
		// 	now, c.Name(), flit.ID)
		m.logFlitE2ETask(flit, true)

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) assemble() bool {
	madeProgress := false

	e := m.assemblingMsgs.Front()
	for e != nil {
		assemblingMsg := e.Value.(*msgToAssemble)

		next := e.Next()

		if assemblingMsg.numFlitArrived < assemblingMsg.numFlitRequired {
			e = next
			continue
		}

		m.assembledMsgs = append(m.assembledMsgs, assemblingMsg.msg)
		m.assemblingMsgs.Remove(e)
		delete(m.assemblingMsgTable, assemblingMsg.msg.Meta().ID)

		e = next

		// fmt.Printf("%.10f, %s, assembled, msg-%s\n",
		// 	c.Engine.CurrentTime(), c.Name(), assemblingMsg.msg.Meta().ID)

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) tryDeliver() bool {
	madeProgress := false

	for len(m.assembledMsgs) > 0 {
		msg := m.assembledMsgs[0]
		dst := msg.Meta().Dst

		var dstPort sim.Port

		for _, port := range m.DevicePorts {
			if port.AsRemote() == dst {
				dstPort = port
				break
			}
		}

		if dstPort == nil {
			panic(fmt.Sprintf("no dst port found for %s", dst))
		}

		err := dstPort.Deliver(msg)
		if err != nil {
			return madeProgress
		}

		// fmt.Printf("%.10f, %s, delivered, %s\n",
		// 	now, c.Name(), msg.Meta().ID)
		m.logMsgE2ETask(msg, true)

		m.assembledMsgs = m.assembledMsgs[1:]

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) logFlitE2ETask(flit *messaging.Flit, isEnd bool) {
	if m.Comp.NumHooks() == 0 {
		return
	}

	msg := flit.Msg

	if isEnd {
		tracing.EndTask(m.flitTaskID(flit), m.Comp)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(flit), m.msgTaskID(msg.Meta().ID),
		m.Comp, "flit_e2e", "flit_e2e", m.Comp.Name()+".FlitBuf", flit,
	)
}

func (m *middleware) logMsgE2ETask(msg sim.Msg, isEnd bool) {
	if m.Comp.NumHooks() == 0 {
		return
	}

	rsp, isRsp := msg.(sim.Rsp)
	if isRsp {
		m.logMsgRsp(isEnd, rsp)
		return
	}

	m.logMsgReq(isEnd, msg)
}

func (m *middleware) logMsgReq(isEnd bool, msg sim.Msg) {
	if isEnd {
		tracing.EndTask(m.msgTaskID(msg.Meta().ID), m.Comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(msg.Meta().ID),
			msg.Meta().ID+"_req_out",
			m.Comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *middleware) logMsgRsp(isEnd bool, rsp sim.Rsp) {
	if isEnd {
		tracing.EndTask(m.msgTaskID(rsp.Meta().ID), m.Comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(rsp.Meta().ID),
			rsp.GetRspTo()+"_req_out",
			m.Comp, "msg_e2e", "msg_e2e", rsp,
		)
	}
}

func (m *middleware) msgToFlits(msg sim.Msg) []*messaging.Flit {
	numFlit := 1

	if msg.Meta().TrafficBytes > 0 {
		trafficByte := msg.Meta().TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * m.encodingOverhead))
		numFlit = (trafficByte-1)/m.flitByteSize + 1
	}

	flits := make([]*messaging.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = messaging.FlitBuilder{}.
			WithSrc(m.NetworkPort.AsRemote()).
			WithDst(m.DefaultSwitchDst).
			WithSeqID(i).
			WithNumFlitInMsg(numFlit).
			WithMsg(msg).
			Build()
	}

	return flits
}
