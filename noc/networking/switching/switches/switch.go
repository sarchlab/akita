// Package switches provides implementations of Switches.
package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v4/noc/messaging"
	"github.com/sarchlab/akita/v4/noc/networking/arbitration"
	"github.com/sarchlab/akita/v4/noc/networking/routing"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type flitPipelineItem struct {
	taskID string
	flit   *messaging.Flit
}

func (f flitPipelineItem) TaskID() string {
	return f.taskID
}

// A portComplex is the infrastructure related to a port.
type portComplex struct {
	// localPort is the port that is equipped on the switch.
	localPort sim.Port

	// remotePort is the port that is connected to the localPort.
	remotePort sim.Port

	// Data arrived at the local port needs to be processed in a pipeline. There
	// is a processing pipeline for each local port.
	pipeline pipelining.Pipeline

	// The flits here are buffered after the pipeline and are waiting to be
	// assigned with an output buffer.
	routeBuffer sim.Buffer

	// The flits here are buffered to wait to be forwarded to the output buffer.
	forwardBuffer sim.Buffer

	// The flits here are waiting to be sent to the next hop.
	sendOutBuffer sim.Buffer

	// NumInputChannel is the number of flits that can stream into the
	// switch from the port. The RouteBuffer and the ForwardBuffer should
	// have the capacity of this number.
	numInputChannel int

	// NumOutputChannel is the number of flits that can stream out of the
	// switch to the port. The SendOutBuffer should have the capacity of this
	// number.
	numOutputChannel int
}

// Comp is an Akita component(Switch) that can forward request to destination.
type Comp struct {
	*sim.TickingComponent

	ports                []sim.Port
	portToComplexMapping map[sim.Port]portComplex
	routingTable         routing.Table
	arbiter              arbitration.Arbiter
}

// addPort adds a new port on the switch.
func (c *Comp) addPort(complex portComplex) {
	c.ports = append(c.ports, complex.localPort)
	c.portToComplexMapping[complex.localPort] = complex
	c.arbiter.AddBuffer(complex.forwardBuffer)
}

// GetRoutingTable returns the routine table used by the switch.
func (c *Comp) GetRoutingTable() routing.Table {
	return c.routingTable
}

// Tick update the Switch's state.
func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.sendOut() || madeProgress
	madeProgress = c.forward() || madeProgress
	madeProgress = c.route() || madeProgress
	madeProgress = c.movePipeline() || madeProgress
	madeProgress = c.startProcessing() || madeProgress

	return madeProgress
}

func (c *Comp) flitParentTaskID(flit *messaging.Flit) string {
	return flit.ID + "_e2e"
}

func (c *Comp) flitTaskID(flit *messaging.Flit) string {
	return flit.ID + "_" + c.Name()
}

func (c *Comp) startProcessing() (madeProgress bool) {
	for _, port := range c.ports {
		pc := c.portToComplexMapping[port]

		for i := 0; i < pc.numInputChannel; i++ {
			item := port.PeekIncoming()
			if item == nil {
				break
			}

			if !pc.pipeline.CanAccept() {
				break
			}

			flit := item.(*messaging.Flit)
			pipelineItem := flitPipelineItem{
				taskID: c.flitTaskID(flit),
				flit:   flit,
			}
			pc.pipeline.Accept(pipelineItem)
			port.RetrieveIncoming()
			madeProgress = true

			tracing.StartTask(
				c.flitTaskID(flit),
				c.flitParentTaskID(flit),
				c, "flit", "flit_inside_sw",
				flit,
			)

			// fmt.Printf("%.10f, %s, switch recv flit, %s\n",
			// 	now, c.Name(), flit.ID)
		}
	}

	return madeProgress
}

func (c *Comp) movePipeline() (madeProgress bool) {
	for _, port := range c.ports {
		pc := c.portToComplexMapping[port]
		madeProgress = pc.pipeline.Tick() || madeProgress
	}

	return madeProgress
}

func (c *Comp) route() (madeProgress bool) {
	for _, port := range c.ports {
		pc := c.portToComplexMapping[port]
		routeBuf := pc.routeBuffer
		forwardBuf := pc.forwardBuffer

		for i := 0; i < pc.numInputChannel; i++ {
			item := routeBuf.Peek()
			if item == nil {
				break
			}

			if !forwardBuf.CanPush() {
				break
			}

			pipelineItem := item.(flitPipelineItem)
			flit := pipelineItem.flit
			c.assignFlitOutputBuf(flit)
			routeBuf.Pop()
			forwardBuf.Push(flit)
			madeProgress = true

			// fmt.Printf("%.10f, %s, switch route flit, %s\n",
			// 	c.Engine.CurrentTime(), c.Name(), flit.ID)
		}
	}

	return madeProgress
}

func (c *Comp) forward() (madeProgress bool) {
	inputBuffers := c.arbiter.Arbitrate()

	for _, buf := range inputBuffers {
		for {
			item := buf.Peek()
			if item == nil {
				break
			}

			flit := item.(*messaging.Flit)
			if !flit.OutputBuf.CanPush() {
				break
			}

			flit.OutputBuf.Push(flit)
			buf.Pop()
			madeProgress = true

			// fmt.Printf("%.10f, %s, switch forward flit, %s\n",
			// 	now, c.Name(), item.(*messaging.Flit).ID)
		}
	}

	return madeProgress
}

func (c *Comp) sendOut() (madeProgress bool) {
	for _, port := range c.ports {
		pc := c.portToComplexMapping[port]
		sendOutBuf := pc.sendOutBuffer

		for i := 0; i < pc.numOutputChannel; i++ {
			item := sendOutBuf.Peek()
			if item == nil {
				break
			}

			flit := item.(*messaging.Flit)
			flit.Meta().Src = pc.localPort
			flit.Meta().Dst = pc.remotePort

			err := pc.localPort.Send(flit)
			if err == nil {
				sendOutBuf.Pop()
				madeProgress = true

				// fmt.Printf("%.10f, %s, switch send flit out, %s\n",
				// 	now, c.Name(), flit.ID)

				tracing.EndTask(c.flitTaskID(flit), c)
			}
		}
	}

	return madeProgress
}

func (c *Comp) assignFlitOutputBuf(f *messaging.Flit) {
	outPort := c.routingTable.FindPort(f.Msg.Meta().Dst)
	if outPort == nil {
		panic(fmt.Sprintf("%s: no output port for %s",
			c.Name(), f.Msg.Meta().Dst))
	}

	pc := c.portToComplexMapping[outPort]

	f.OutputBuf = pc.sendOutBuffer
	if f.OutputBuf == nil {
		panic(fmt.Sprintf("%s: no output buffer for %s",
			c.Name(), f.Msg.Meta().Dst))
	}
}

// SwitchPortAdder can add a port to a switch.
type SwitchPortAdder struct {
	sw               *Comp
	localPort        sim.Port
	remotePort       sim.Port
	latency          int
	numInputChannel  int
	numOutputChannel int
}

// MakeSwitchPortAdder creates a SwitchPortAdder that can add ports for the
// provided switch.
func MakeSwitchPortAdder(sw *Comp) SwitchPortAdder {
	return SwitchPortAdder{
		sw:               sw,
		numInputChannel:  1,
		numOutputChannel: 1,
		latency:          1,
	}
}

// WithPorts defines the ports to add. The local port is part of the switch.
// The remote port is the port on an endpoint or on another switch.
func (a SwitchPortAdder) WithPorts(local, remote sim.Port) SwitchPortAdder {
	a.localPort = local
	a.remotePort = remote
	return a
}

// WithLatency sets the latency of the port.
func (a SwitchPortAdder) WithLatency(latency int) SwitchPortAdder {
	a.latency = latency
	return a
}

// WithNumInputChannel sets the number of input channels of the port. This
// number determines the number of flits that can be injected into the switch
// from the port in each cycle.
func (a SwitchPortAdder) WithNumInputChannel(num int) SwitchPortAdder {
	a.numInputChannel = num
	return a
}

// WithNumOutputChannel sets the number of output channels of the port. This
// number determines the number of flits that can be ejected from the switch
// to the port in each cycle.
func (a SwitchPortAdder) WithNumOutputChannel(num int) SwitchPortAdder {
	a.numOutputChannel = num
	return a
}

// AddPort adds the port to the switch.
func (a SwitchPortAdder) AddPort() {
	complexID := len(a.sw.ports)
	complexName := fmt.Sprintf("%s.PortComplex%d", a.sw.Name(), complexID)

	sendOutBuf := sim.NewBuffer(complexName+"SendOutBuf", a.numOutputChannel)
	forwardBuf := sim.NewBuffer(complexName+"ForwardBuf", a.numInputChannel)
	routeBuf := sim.NewBuffer(complexName+"RouteBuf", a.numInputChannel)
	pipeline := pipelining.MakeBuilder().
		WithNumStage(a.latency).
		WithCyclePerStage(1).
		WithPipelineWidth(a.numInputChannel).
		WithPostPipelineBuffer(routeBuf).
		Build(a.localPort.Name() + ".Pipeline")

	pc := portComplex{
		localPort:        a.localPort,
		remotePort:       a.remotePort,
		pipeline:         pipeline,
		routeBuffer:      routeBuf,
		forwardBuffer:    forwardBuf,
		sendOutBuffer:    sendOutBuf,
		numInputChannel:  a.numInputChannel,
		numOutputChannel: a.numOutputChannel,
	}

	a.sw.addPort(pc)
}
