// Package endpoint provides endpoint
package endpoint

import (
	"container/list"
	"fmt"
	"math"

	"github.com/sarchlab/akita/v3/noc/messaging"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
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
	msgOutBufSize     int
	flitsToSend       []*messaging.Flit

	assemblingMsgTable map[string]*list.Element
	assemblingMsgs     *list.List
	assembledMsgs      []sim.Msg
}

// CanSend returns whether the endpoint can send a message.
func (c *Comp) CanSend(_ sim.Port) bool {
	c.Lock()
	defer c.Unlock()

	return len(c.msgOutBuf) < c.msgOutBufSize
}

// Send initiates a message sending process. It breaks down the message into
// flits and send the flits to the external connections.
func (c *Comp) Send(msg sim.Msg) *sim.SendError {
	c.Lock()
	defer c.Unlock()

	if len(c.msgOutBuf) >= c.msgOutBufSize {
		return &sim.SendError{}
	}

	c.msgOutBuf = append(c.msgOutBuf, msg)

	c.TickLater(msg.Meta().SendTime)

	c.logMsgE2ETask(msg, false)

	return nil
}

// PlugIn connects a port to the endpoint.
func (c *Comp) PlugIn(port sim.Port, srcBufCap int) {
	port.SetConnection(c)
	c.DevicePorts = append(c.DevicePorts, port)
	c.msgOutBufSize = srcBufCap
}

// NotifyAvailable triggers the endpoint to continue to tick.
func (c *Comp) NotifyAvailable(now sim.VTimeInSec, _ sim.Port) {
	c.TickLater(now)
}

// Unplug removes the association of a port and an endpoint.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

// Tick update the endpoint state.
func (c *Comp) Tick(now sim.VTimeInSec) bool {
	c.Lock()
	defer c.Unlock()

	madeProgress := false

	madeProgress = c.sendFlitOut(now) || madeProgress
	madeProgress = c.prepareFlits(now) || madeProgress
	madeProgress = c.tryDeliver(now) || madeProgress
	madeProgress = c.assemble(now) || madeProgress
	madeProgress = c.recv(now) || madeProgress

	return madeProgress
}

func (c *Comp) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (c *Comp) flitTaskID(flit sim.Msg) string {
	return fmt.Sprintf("%s_e2e", flit.Meta().ID)
}

func (c *Comp) sendFlitOut(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < c.numOutputChannels; i++ {
		if len(c.flitsToSend) == 0 {
			return madeProgress
		}

		flit := c.flitsToSend[0]
		flit.SendTime = now
		err := c.NetworkPort.Send(flit)

		if err == nil {
			c.flitsToSend = c.flitsToSend[1:]

			// fmt.Printf("%.10f, %s, ep send, %s, %d\n",
			// 	ep.Engine.CurrentTime(), ep.Name(),
			// 	flit.Meta().ID, len(ep.flitsToSend))

			if len(c.flitsToSend) == 0 {
				for _, p := range c.DevicePorts {
					p.NotifyAvailable(now)
				}
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (c *Comp) prepareFlits(_ sim.VTimeInSec) bool {
	madeProgress := false

	for {
		if len(c.msgOutBuf) == 0 {
			return madeProgress
		}

		if len(c.msgOutBuf) > c.numOutputChannels {
			return madeProgress
		}

		msg := c.msgOutBuf[0]
		c.msgOutBuf = c.msgOutBuf[1:]
		c.flitsToSend = append(c.flitsToSend, c.msgToFlits(msg)...)

		// fmt.Printf("%.10f, %s, ep send, msg-%s, %d\n",
		// 	ep.Engine.CurrentTime(), ep.Name(), msg.Meta().ID,
		// 	len(ep.flitsToSend))

		for _, flit := range c.flitsToSend {
			c.logFlitE2ETask(flit, false)
		}

		madeProgress = true
	}
}

func (c *Comp) recv(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < c.numInputChannels; i++ {
		received := c.NetworkPort.Peek()
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

		c.NetworkPort.Retrieve(now)

		c.logFlitE2ETask(flit, true)

		madeProgress = true

		// fmt.Printf("%.10f, %s, ep received flit %s\n",
		// 	now, ep.Name(), flit.ID)
	}

	return madeProgress
}

func (c *Comp) assemble(_ sim.VTimeInSec) bool {
	madeProgress := false

	for e := c.assemblingMsgs.Front(); e != nil; e = e.Next() {
		assemblingMsg := e.Value.(*msgToAssemble)

		if assemblingMsg.numFlitArrived < assemblingMsg.numFlitRequired {
			continue
		}

		c.assembledMsgs = append(c.assembledMsgs, assemblingMsg.msg)
		c.assemblingMsgs.Remove(e)
		delete(c.assemblingMsgTable, assemblingMsg.msg.Meta().ID)

		madeProgress = true
	}

	return madeProgress
}

func (c *Comp) tryDeliver(now sim.VTimeInSec) bool {
	madeProgress := false

	for len(c.assembledMsgs) > 0 {
		msg := c.assembledMsgs[0]
		msg.Meta().RecvTime = now

		err := msg.Meta().Dst.Recv(msg)
		if err != nil {
			return madeProgress
		}

		// fmt.Printf("%.10f, %s, delivered, %s\n",
		// 	now, ep.Name(), msg.Meta().ID)
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

// // EndPointBuilder can build End Points.
// type EndPointBuilder struct {
// 	engine                   sim.Engine
// 	freq                     sim.Freq
// 	numInputChannels         int
// 	numOutputChannels        int
// 	flitByteSize             int
// 	encodingOverhead         float64
// 	flitAssemblingBufferSize int
// 	networkPortBufferSize    int
// 	devicePorts              []sim.Port
// }

// // MakeEndPointBuilder creates a new EndPointBuilder with default
// // configurations.
// func MakeEndPointBuilder() EndPointBuilder {
// 	return EndPointBuilder{
// 		flitByteSize:             32,
// 		flitAssemblingBufferSize: 64,
// 		networkPortBufferSize:    4,
// 		freq:                     1 * sim.GHz,
// 		numInputChannels:         1,
// 		numOutputChannels:        1,
// 	}
// }

// // WithEngine sets the engine of the End Point to build.
// func (b EndPointBuilder) WithEngine(e sim.Engine) EndPointBuilder {
// 	b.engine = e
// 	return b
// }

// // WithFreq sets the frequency of the End Point to built.
// func (b EndPointBuilder) WithFreq(freq sim.Freq) EndPointBuilder {
// 	b.freq = freq
// 	return b
// }

// // WithNumInputChannels sets the number of input channels of the End Point
// // to build.
// func (b EndPointBuilder) WithNumInputChannels(num int) EndPointBuilder {
// 	b.numInputChannels = num
// 	return b
// }

// // WithNumOutputChannels sets the number of output channels of the End Point
// // to build.
// func (b EndPointBuilder) WithNumOutputChannels(num int) EndPointBuilder {
// 	b.numOutputChannels = num
// 	return b
// }

// // WithFlitByteSize sets the flit byte size that the End Point supports.
// func (b EndPointBuilder) WithFlitByteSize(n int) EndPointBuilder {
// 	b.flitByteSize = n
// 	return b
// }

// // WithEncodingOverhead sets the encoding overhead.
// func (b EndPointBuilder) WithEncodingOverhead(o float64) EndPointBuilder {
// 	b.encodingOverhead = o
// 	return b
// }

// // WithNetworkPortBufferSize sets the network port buffer size of the end point.
// func (b EndPointBuilder) WithNetworkPortBufferSize(n int) EndPointBuilder {
// 	b.networkPortBufferSize = n
// 	return b
// }

// // WithDevicePorts sets a list of ports that communicate directly through the
// // End Point.
// func (b EndPointBuilder) WithDevicePorts(ports []sim.Port) EndPointBuilder {
// 	b.devicePorts = ports
// 	return b
// }

// // Build creates a new End Point.
// func (b EndPointBuilder) Build(name string) *EndPoint {
// 	b.engineMustBeGiven()
// 	b.freqMustBeGiven()
// 	b.flitByteSizeMustBeGiven()

// 	ep := &EndPoint{}
// 	ep.TickingComponent = sim.NewTickingComponent(
// 		name, b.engine, b.freq, ep)
// 	ep.flitByteSize = b.flitByteSize

// 	ep.numInputChannels = b.numInputChannels
// 	ep.numOutputChannels = b.numOutputChannels

// 	ep.assemblingMsgs = list.New()
// 	ep.assemblingMsgTable = make(map[string]*list.Element)

// 	ep.NetworkPort = sim.NewLimitNumMsgPort(
// 		ep, b.networkPortBufferSize,
// 		fmt.Sprintf("%s.NetworkPort", ep.Name()))

// 	for _, dp := range b.devicePorts {
// 		ep.PlugIn(dp, 1)
// 	}

// 	return ep
// }

// func (b EndPointBuilder) engineMustBeGiven() {
// 	if b.engine == nil {
// 		panic("engine is not given")
// 	}
// }

// func (b EndPointBuilder) freqMustBeGiven() {
// 	if b.freq == 0 {
// 		panic("freq must be given")
// 	}
// }

// func (b EndPointBuilder) flitByteSizeMustBeGiven() {
// 	if b.flitByteSize == 0 {
// 		panic("flit byte size must be given")
// 	}
// }
