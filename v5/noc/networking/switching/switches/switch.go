// Package switches provides implementations of Switches.
package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the switch.
type Spec struct {
	Freq sim.Freq `json:"freq"`
}

// routedFlit is a flit that has been received and assigned a route destination.
type routedFlit struct {
	messaging.Flit
	TaskID  string         `json:"task_id"`
	RouteTo sim.RemotePort `json:"route_to"`
}

// portComplexState is the serializable state of one port complex.
type portComplexState struct {
	LocalPortName    string               `json:"local_port_name"`
	RemotePort       sim.RemotePort       `json:"remote_port"`
	NumInputChannel  int                  `json:"num_input_channel"`
	NumOutputChannel int                  `json:"num_output_channel"`
	Latency          int                  `json:"latency"`
	PipelineWidth    int                  `json:"pipeline_width"`
	Pipeline         stateutil.Pipeline[routedFlit]       `json:"pipeline"`
	RouteBuffer      stateutil.Buffer[routedFlit]         `json:"route_buffer"`
	ForwardBuffer    stateutil.Buffer[routedFlit]         `json:"forward_buffer"`
	SendOutBuffer    stateutil.Buffer[messaging.Flit]     `json:"send_out_buffer"`
}

// State contains mutable runtime data for the switch.
type State struct {
	PortComplexes []portComplexState `json:"port_complexes"`
}

// --- Comp ---

// Comp is an Akita component(Switch) that can forward request to destination.
type Comp struct {
	*modeling.Component[Spec, State]
}

// routeForwardSendMiddleware returns the routeForwardSendMW from the
// component's middleware list (registered at index 0).
func (c *Comp) routeForwardSendMiddleware() *routeForwardSendMW {
	return c.Middlewares()[0].(*routeForwardSendMW)
}

// GetRoutingTable returns the routine table used by the switch.
func (c *Comp) GetRoutingTable() routing.Table {
	return c.routeForwardSendMiddleware().routingTable
}

// --- routeForwardSendMW ---

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

			item, _ := pcs.RouteBuffer.PopTyped()
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
	occupiedOutputPort := make(map[int]bool)

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

			pcs.ForwardBuffer.PopTyped()
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

			fm := curPcs.SendOutBuffer.Elements[numSent]
			flit := &messaging.Flit{
				MsgMeta:      fm.MsgMeta,
				SeqID:        fm.SeqID,
				NumFlitInMsg: fm.NumFlitInMsg,
				Msg:          fm.Msg,
			}
			flit.Src = port.AsRemote()
			flit.Dst = curPcs.RemotePort

			err := port.Send(flit)
			if err == nil {
				madeProgress = true
				numSent++

				tracing.EndTask(m.flitTaskID(flit), m.comp)
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

// --- receivePipelineMW ---

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

// addPort registers a port complex.
func addPort(
	comp *modeling.Component[Spec, State],
	ports *[]sim.Port,
	portIndex map[sim.RemotePort]int,
	port sim.Port,
	remotePort sim.RemotePort,
	pcs portComplexState,
) {
	idx := len(*ports)
	*ports = append(*ports, port)
	portIndex[remotePort] = idx

	// Also map the local port's RemotePort so route resolution works
	portIndex[port.AsRemote()] = idx

	// Initialize stateutil.Buffer fields
	pcs.RouteBuffer = stateutil.Buffer[routedFlit]{
		BufferName: pcs.LocalPortName + "RouteBuf",
		Cap:        pcs.NumInputChannel,
	}
	pcs.ForwardBuffer = stateutil.Buffer[routedFlit]{
		BufferName: pcs.LocalPortName + "FwdBuf",
		Cap:        pcs.NumInputChannel,
	}
	pcs.SendOutBuffer = stateutil.Buffer[messaging.Flit]{
		BufferName: pcs.LocalPortName + "SendBuf",
		Cap:        pcs.NumOutputChannel,
	}
	pcs.Pipeline = stateutil.Pipeline[routedFlit]{
		Width:     pcs.PipelineWidth,
		NumStages: pcs.Latency,
	}

	// Initialize state in both current and next buffers
	next := comp.GetNextState()
	next.PortComplexes = append(next.PortComplexes, pcs)
	comp.SetState(*next)
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
	pcs := portComplexState{
		LocalPortName:    a.localPort.Name(),
		RemotePort:       a.remotePort.AsRemote(),
		NumInputChannel:  a.numInputChannel,
		NumOutputChannel: a.numOutputChannel,
		Latency:          a.latency,
		PipelineWidth:    a.numInputChannel,
	}
	rfsMW := a.sw.routeForwardSendMiddleware()
	addPort(rfsMW.comp, &rfsMW.ports, rfsMW.portIndex,
		a.localPort, a.remotePort.AsRemote(), pcs)

	// Keep receivePipelineMW's ports/portIndex in sync
	rpMW := a.sw.Middlewares()[1].(*receivePipelineMW)
	rpMW.ports = rfsMW.ports
	rpMW.portIndex = rfsMW.portIndex
}
