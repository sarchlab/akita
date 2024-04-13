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

// Comp is an akita component(Endpoint) that delegates sending and receiving actions
// of a few ports.
type Comp struct {
	*sim.TickingComponent

	DevicePorts      []sim.Port
	NetworkPort      sim.Port
	DefaultSwitchDst sim.Port

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
func (c *Comp) PlugIn(port sim.Port, srcBufCap int) {
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

// Tick update the endpoint state.
func (c *Comp) Tick() bool {
	c.Lock()
	defer c.Unlock()

	madeProgress := false

	madeProgress = c.sendFlitOut() || madeProgress
	madeProgress = c.prepareMsg() || madeProgress
	madeProgress = c.prepareFlits() || madeProgress
	madeProgress = c.tryDeliver() || madeProgress
	madeProgress = c.assemble() || madeProgress
	madeProgress = c.recv() || madeProgress

	return madeProgress
}

func (c *Comp) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (c *Comp) flitTaskID(flit sim.Msg) string {
	return fmt.Sprintf("%s_e2e", flit.Meta().ID)
}

func (c *Comp) sendFlitOut() bool {
	madeProgress := false

	for i := 0; i < c.numOutputChannels; i++ {
		if len(c.flitsToSend) == 0 {
			return madeProgress
		}

		flit := c.flitsToSend[0]
		err := c.NetworkPort.Send(flit)

		if err == nil {
			c.flitsToSend = c.flitsToSend[1:]

			// fmt.Printf("%.10f, %s, ep send flit, %s, %d\n",
			// 	c.Engine.CurrentTime(), c.Name(),
			// 	flit.Meta().ID, len(c.flitsToSend))

			if len(c.flitsToSend) == 0 {
				for _, p := range c.DevicePorts {
					p.NotifyAvailable()
				}
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (c *Comp) prepareMsg() bool {
	madeProgress := false
	for i := 0; i < len(c.DevicePorts); i++ {
		port := c.DevicePorts[i]
		if port.PeekOutgoing() == nil {
			continue
		}

		msg := port.RetrieveOutgoing()
		c.msgOutBuf = append(c.msgOutBuf, msg)

		// fmt.Printf("%.10f, %s, ep send msg, msg-%s\n",
		// 	now, c.Name(), msg.Meta().ID)

		madeProgress = true
	}

	return madeProgress
}

func (c *Comp) prepareFlits() bool {
	madeProgress := false

	for {
		if len(c.msgOutBuf) == 0 {
			return madeProgress
		}

		msg := c.msgOutBuf[0]
		c.msgOutBuf = c.msgOutBuf[1:]
		flits := c.msgToFlits(msg)
		c.flitsToSend = append(c.flitsToSend, flits...)

		// fmt.Printf("%.10f, %s, ep create flit, msg-%s, %d, %d\n",
		// 	c.Engine.CurrentTime(), c.Name(), msg.Meta().ID, len(flits),
		// 	len(c.flitsToSend))

		for _, flit := range flits {
			c.logFlitE2ETask(flit, false)
		}

		madeProgress = true
	}
}

func (c *Comp) recv() bool {
	madeProgress := false

	for i := 0; i < c.numInputChannels; i++ {
		received := c.NetworkPort.PeekIncoming()
		if received == nil {
			return madeProgress
		}

		flit := received.(*messaging.Flit)
		msg := flit.Msg

		assemblingElem := c.assemblingMsgTable[msg.Meta().ID]
		if assemblingElem == nil {
			assemblingElem = c.assemblingMsgs.PushBack(&msgToAssemble{
				msg:             msg,
				numFlitRequired: flit.NumFlitInMsg,
				numFlitArrived:  0,
			})
			c.assemblingMsgTable[msg.Meta().ID] = assemblingElem
		}

		assembling := assemblingElem.Value.(*msgToAssemble)
		assembling.numFlitArrived++

		c.NetworkPort.RetrieveIncoming()

		c.logFlitE2ETask(flit, true)

		madeProgress = true

		// fmt.Printf("%.10f, %s, ep received flit %s\n",
		// 	now, c.Name(), flit.ID)
	}

	return madeProgress
}

func (c *Comp) assemble() bool {
	madeProgress := false

	e := c.assemblingMsgs.Front()
	for e != nil {
		assemblingMsg := e.Value.(*msgToAssemble)

		next := e.Next()

		if assemblingMsg.numFlitArrived < assemblingMsg.numFlitRequired {
			e = next
			continue
		}

		c.assembledMsgs = append(c.assembledMsgs, assemblingMsg.msg)
		c.assemblingMsgs.Remove(e)
		delete(c.assemblingMsgTable, assemblingMsg.msg.Meta().ID)

		e = next

		// fmt.Printf("%.10f, %s, assembled, msg-%s\n",
		// 	c.Engine.CurrentTime(), c.Name(), assemblingMsg.msg.Meta().ID)

		madeProgress = true
	}

	return madeProgress
}

func (c *Comp) tryDeliver() bool {
	madeProgress := false

	for len(c.assembledMsgs) > 0 {
		msg := c.assembledMsgs[0]

		err := msg.Meta().Dst.Deliver(msg)
		if err != nil {
			return madeProgress
		}

		// fmt.Printf("%.10f, %s, delivered, %s\n",
		// 	now, c.Name(), msg.Meta().ID)
		c.logMsgE2ETask(msg, true)

		c.assembledMsgs = c.assembledMsgs[1:]

		madeProgress = true
	}

	return madeProgress
}

func (c *Comp) logFlitE2ETask(flit *messaging.Flit, isEnd bool) {
	if c.NumHooks() == 0 {
		return
	}

	msg := flit.Msg

	if isEnd {
		tracing.EndTask(c.flitTaskID(flit), c)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		c.flitTaskID(flit), c.msgTaskID(msg.Meta().ID),
		c, "flit_e2e", "flit_e2e", c.Name()+".FlitBuf", flit,
	)
}

func (c *Comp) logMsgE2ETask(msg sim.Msg, isEnd bool) {
	if c.NumHooks() == 0 {
		return
	}

	rsp, isRsp := msg.(sim.Rsp)
	if isRsp {
		c.logMsgRsp(isEnd, rsp)
		return
	}

	c.logMsgReq(isEnd, msg)
}

func (c *Comp) logMsgReq(isEnd bool, msg sim.Msg) {
	if isEnd {
		tracing.EndTask(c.msgTaskID(msg.Meta().ID), c)
	} else {
		tracing.StartTask(
			c.msgTaskID(msg.Meta().ID),
			msg.Meta().ID+"_req_out",
			c, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (c *Comp) logMsgRsp(isEnd bool, rsp sim.Rsp) {
	if isEnd {
		tracing.EndTask(c.msgTaskID(rsp.Meta().ID), c)
	} else {
		tracing.StartTask(
			c.msgTaskID(rsp.Meta().ID),
			rsp.GetRspTo()+"_req_out",
			c, "msg_e2e", "msg_e2e", rsp,
		)
	}
}

func (c *Comp) msgToFlits(msg sim.Msg) []*messaging.Flit {
	numFlit := 1
	if msg.Meta().TrafficBytes > 0 {
		trafficByte := msg.Meta().TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * c.encodingOverhead))
		numFlit = (trafficByte-1)/c.flitByteSize + 1
	}

	flits := make([]*messaging.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = messaging.FlitBuilder{}.
			WithSrc(c.NetworkPort).
			WithDst(c.DefaultSwitchDst).
			WithSeqID(i).
			WithNumFlitInMsg(numFlit).
			WithMsg(msg).
			Build()
	}

	return flits
}
