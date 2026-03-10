// Package switches provides implementations of Switches.
package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/arbitration"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type flitPipelineItem struct {
	taskID string
	flit   *sim.Msg // payload: *messaging.FlitPayload
}

func (f flitPipelineItem) TaskID() string {
	return f.taskID
}

// A portComplex is the infrastructure related to a port.
type portComplex struct {
	// localPort is the port that is equipped on the switch.
	localPort sim.Port

	// remotePort is the port that is connected to the localPort.
	remotePort sim.RemotePort

	// Data arrived at the local port needs to be processed in a pipeline. There
	// is a processing pipeline for each local port.
	pipeline queueing.Pipeline

	// The flits here are buffered after the pipeline and are waiting to be
	// assigned with an output buffer.
	routeBuffer queueing.Buffer

	// The flits here are buffered to wait to be forwarded to the output buffer.
	forwardBuffer queueing.Buffer

	// The flits here are waiting to be sent to the next hop.
	sendOutBuffer queueing.Buffer

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
	sim.MiddlewareHolder

	ports                []sim.Port
	portToComplexMapping map[sim.RemotePort]portComplex
	routingTable         routing.Table
	arbiter              arbitration.Arbiter
}

// addPort adds a new port on the switch.
func (c *Comp) addPort(complex portComplex) {
	c.ports = append(c.ports, complex.localPort)
	c.portToComplexMapping[complex.localPort.AsRemote()] = complex
	c.arbiter.AddBuffer(complex.forwardBuffer)
}

// GetRoutingTable returns the routine table used by the switch.
func (c *Comp) GetRoutingTable() routing.Table {
	return c.routingTable
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick update the Switch's state.
func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendOut() || madeProgress
	madeProgress = m.forward() || madeProgress
	madeProgress = m.route() || madeProgress
	madeProgress = m.movePipeline() || madeProgress
	madeProgress = m.startProcessing() || madeProgress

	return madeProgress
}

func (m *middleware) flitParentTaskID(flitMsg *sim.Msg) string {
	return flitMsg.ID + "_e2e"
}

func (m *middleware) flitTaskID(flitMsg *sim.Msg) string {
	return flitMsg.ID + "_" + m.Comp.Name()
}

func (m *middleware) startProcessing() (madeProgress bool) {
	for _, port := range m.ports {
		pc := m.portToComplexMapping[port.AsRemote()]

		for i := 0; i < pc.numInputChannel; i++ {
			item := port.PeekIncoming()
			if item == nil {
				break
			}

			if !pc.pipeline.CanAccept() {
				break
			}

			pipelineItem := flitPipelineItem{
				taskID: m.flitTaskID(item),
				flit:   item,
			}
			pc.pipeline.Accept(pipelineItem)
			port.RetrieveIncoming()

			madeProgress = true

			tracing.StartTask(
				m.flitTaskID(item),
				m.flitParentTaskID(item),
				m.Comp, "flit", "flit_inside_sw",
				item,
			)
		}
	}

	return madeProgress
}

func (m *middleware) movePipeline() (madeProgress bool) {
	for _, port := range m.ports {
		pc := m.portToComplexMapping[port.AsRemote()]
		madeProgress = pc.pipeline.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) route() (madeProgress bool) {
	for _, port := range m.ports {
		pc := m.portToComplexMapping[port.AsRemote()]
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
			flitMsg := pipelineItem.flit
			flitPayload := sim.MsgPayload[messaging.FlitPayload](flitMsg)
			m.assignFlitOutputBuf(flitMsg, flitPayload)
			routeBuf.Pop()
			forwardBuf.Push(flitMsg)

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) forward() (madeProgress bool) {
	inputBuffers := m.arbiter.Arbitrate()

	for _, buf := range inputBuffers {
		for {
			item := buf.Peek()
			if item == nil {
				break
			}

			flitMsg := item.(*sim.Msg)
			flitPayload := sim.MsgPayload[messaging.FlitPayload](flitMsg)
			if !flitPayload.OutputBuf.CanPush() {
				break
			}

			flitPayload.OutputBuf.Push(flitMsg)
			buf.Pop()

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) sendOut() (madeProgress bool) {
	for _, port := range m.ports {
		pc := m.portToComplexMapping[port.AsRemote()]
		sendOutBuf := pc.sendOutBuffer

		for i := 0; i < pc.numOutputChannel; i++ {
			item := sendOutBuf.Peek()
			if item == nil {
				break
			}

			flitMsg := item.(*sim.Msg)
			flitMsg.Src = pc.localPort.AsRemote()
			flitMsg.Dst = pc.remotePort

			err := pc.localPort.Send(flitMsg)
			if err == nil {
				madeProgress = true

				sendOutBuf.Pop()

				tracing.EndTask(m.flitTaskID(flitMsg), m.Comp)
			}
		}
	}

	return madeProgress
}

func (m *middleware) assignFlitOutputBuf(
	flitMsg *sim.Msg,
	flitPayload *messaging.FlitPayload,
) {
	outPort := m.routingTable.FindPort(flitPayload.Msg.Dst)
	if outPort == "" {
		panic(fmt.Sprintf("%s: no output port for %s",
			m.Comp.Name(), flitPayload.Msg.Dst))
	}

	pc := m.portToComplexMapping[outPort]

	flitPayload.OutputBuf = pc.sendOutBuffer
	if flitPayload.OutputBuf == nil {
		panic(fmt.Sprintf("%s: no output buffer for %s",
			m.Comp.Name(), flitPayload.Msg.Dst))
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

	sendOutBuf := queueing.NewBuffer(complexName+"SendOutBuf", a.numOutputChannel)
	forwardBuf := queueing.NewBuffer(complexName+"ForwardBuf", a.numInputChannel)
	routeBuf := queueing.NewBuffer(complexName+"RouteBuf", a.numInputChannel)
	pipeline := queueing.MakeBuilder().
		WithNumStage(a.latency).
		WithCyclePerStage(1).
		WithPipelineWidth(a.numInputChannel).
		WithPostPipelineBuffer(routeBuf).
		Build(a.localPort.Name() + ".Pipeline")

	pc := portComplex{
		localPort:        a.localPort,
		remotePort:       a.remotePort.AsRemote(),
		pipeline:         pipeline,
		routeBuffer:      routeBuf,
		forwardBuffer:    forwardBuf,
		sendOutBuffer:    sendOutBuf,
		numInputChannel:  a.numInputChannel,
		numOutputChannel: a.numOutputChannel,
	}

	a.sw.addPort(pc)
}
