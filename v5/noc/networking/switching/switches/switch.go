// Package switches provides implementations of Switches.
package switches

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/arbitration"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the switch.
type Spec struct{}

// msgRef is a serializable representation of a sim.Msg.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// flitPipelineItemState is a serializable flit pipeline item.
type flitPipelineItemState struct {
	TaskID string `json:"task_id"`
	Flit   msgRef `json:"flit"`
}

// pipelineStageState captures one non-nil pipeline slot.
type pipelineStageState struct {
	Lane      int                   `json:"lane"`
	Stage     int                   `json:"stage"`
	Item      flitPipelineItemState `json:"item"`
	CycleLeft int                   `json:"cycle_left"`
}

// portComplexState is the serializable state of one port complex.
type portComplexState struct {
	LocalPortName  string                  `json:"local_port_name"`
	RemotePort     sim.RemotePort          `json:"remote_port"`
	PipelineStages []pipelineStageState    `json:"pipeline_stages"`
	RouteBuffer    []flitPipelineItemState `json:"route_buffer"`
	ForwardBuffer  []msgRef                `json:"forward_buffer"`
	SendOutBuffer  []msgRef                `json:"send_out_buffer"`
}

// State contains mutable runtime data for the switch.
type State struct {
	PortComplexes []portComplexState `json:"port_complexes"`
}

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
	*modeling.Component[Spec, State]

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

func msgRefFromMsg(msg sim.Msg) msgRef {
	meta := msg.Meta()
	return msgRef{
		ID:           meta.ID,
		Src:          meta.Src,
		Dst:          meta.Dst,
		RspTo:        meta.RspTo,
		TrafficClass: meta.TrafficClass,
		TrafficBytes: meta.TrafficBytes,
	}
}

func msgFromRef(ref msgRef) *sim.GenericMsg {
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           ref.ID,
			Src:          ref.Src,
			Dst:          ref.Dst,
			RspTo:        ref.RspTo,
			TrafficClass: ref.TrafficClass,
			TrafficBytes: ref.TrafficBytes,
		},
	}
}

func flitPipelineItemStateFromItem(item flitPipelineItem) flitPipelineItemState {
	return flitPipelineItemState{
		TaskID: item.taskID,
		Flit:   msgRefFromMsg(item.flit),
	}
}

func flitPipelineItemFromState(s flitPipelineItemState) flitPipelineItem {
	return flitPipelineItem{
		taskID: s.TaskID,
		flit: &messaging.Flit{
			MsgMeta: sim.MsgMeta{
				ID:           s.Flit.ID,
				Src:          s.Flit.Src,
				Dst:          s.Flit.Dst,
				RspTo:        s.Flit.RspTo,
				TrafficClass: s.Flit.TrafficClass,
				TrafficBytes: s.Flit.TrafficBytes,
			},
		},
	}
}

// snapshotState converts runtime mutable data into a serializable State.
func (c *Comp) snapshotState() State {
	s := State{}

	s.PortComplexes = make([]portComplexState, 0, len(c.ports))

	for _, port := range c.ports {
		pc := c.portToComplexMapping[port.AsRemote()]

		pcs := portComplexState{
			LocalPortName: port.Name(),
			RemotePort:    pc.remotePort,
		}

		// Snapshot pipeline
		pipeSnaps := queueing.SnapshotPipeline(pc.pipeline)
		pcs.PipelineStages = make([]pipelineStageState, len(pipeSnaps))
		for i, snap := range pipeSnaps {
			item := snap.Elem.(flitPipelineItem)
			pcs.PipelineStages[i] = pipelineStageState{
				Lane:      snap.Lane,
				Stage:     snap.Stage,
				Item:      flitPipelineItemStateFromItem(item),
				CycleLeft: snap.CycleLeft,
			}
		}

		// Snapshot routeBuffer (holds flitPipelineItem values)
		routeElems := queueing.SnapshotBuffer(pc.routeBuffer)
		pcs.RouteBuffer = make([]flitPipelineItemState, len(routeElems))
		for i, elem := range routeElems {
			item := elem.(flitPipelineItem)
			pcs.RouteBuffer[i] = flitPipelineItemStateFromItem(item)
		}

		// Snapshot forwardBuffer (holds *messaging.Flit)
		fwdElems := queueing.SnapshotBuffer(pc.forwardBuffer)
		pcs.ForwardBuffer = make([]msgRef, len(fwdElems))
		for i, elem := range fwdElems {
			flit := elem.(*messaging.Flit)
			pcs.ForwardBuffer[i] = msgRefFromMsg(flit)
		}

		// Snapshot sendOutBuffer (holds *messaging.Flit)
		sendElems := queueing.SnapshotBuffer(pc.sendOutBuffer)
		pcs.SendOutBuffer = make([]msgRef, len(sendElems))
		for i, elem := range sendElems {
			flit := elem.(*messaging.Flit)
			pcs.SendOutBuffer[i] = msgRefFromMsg(flit)
		}

		s.PortComplexes = append(s.PortComplexes, pcs)
	}

	return s
}

// restoreFromState restores runtime mutable data from a serializable State.
func (c *Comp) restoreFromState(s State) {
	for _, pcs := range s.PortComplexes {
		// Find the matching port complex by iterating ports
		var pc portComplex
		var portKey sim.RemotePort
		found := false

		for _, port := range c.ports {
			if port.Name() == pcs.LocalPortName {
				portKey = port.AsRemote()
				pc = c.portToComplexMapping[portKey]
				found = true
				break
			}
		}

		if !found {
			continue
		}

		// Restore pipeline
		pipeSnaps := make([]queueing.PipelineStageSnapshot, len(pcs.PipelineStages))
		for i, ps := range pcs.PipelineStages {
			pipeSnaps[i] = queueing.PipelineStageSnapshot{
				Lane:      ps.Lane,
				Stage:     ps.Stage,
				Elem:      flitPipelineItemFromState(ps.Item),
				CycleLeft: ps.CycleLeft,
			}
		}
		queueing.RestorePipeline(pc.pipeline, pipeSnaps)

		// Restore routeBuffer
		routeElems := make([]interface{}, len(pcs.RouteBuffer))
		for i, rs := range pcs.RouteBuffer {
			routeElems[i] = flitPipelineItemFromState(rs)
		}
		queueing.RestoreBuffer(pc.routeBuffer, routeElems)

		// Restore forwardBuffer
		fwdElems := make([]interface{}, len(pcs.ForwardBuffer))
		for i, ref := range pcs.ForwardBuffer {
			fwdElems[i] = &messaging.Flit{
				MsgMeta: sim.MsgMeta{
					ID:           ref.ID,
					Src:          ref.Src,
					Dst:          ref.Dst,
					RspTo:        ref.RspTo,
					TrafficClass: ref.TrafficClass,
					TrafficBytes: ref.TrafficBytes,
				},
			}
		}
		queueing.RestoreBuffer(pc.forwardBuffer, fwdElems)

		// Restore sendOutBuffer
		sendElems := make([]interface{}, len(pcs.SendOutBuffer))
		for i, ref := range pcs.SendOutBuffer {
			sendElems[i] = &messaging.Flit{
				MsgMeta: sim.MsgMeta{
					ID:           ref.ID,
					Src:          ref.Src,
					Dst:          ref.Dst,
					RspTo:        ref.RspTo,
					TrafficClass: ref.TrafficClass,
					TrafficBytes: ref.TrafficBytes,
				},
			}
		}
		queueing.RestoreBuffer(pc.sendOutBuffer, sendElems)
	}
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := c.snapshotState()
	c.Component.SetState(state)
	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)
	c.restoreFromState(state)
}

// SaveState marshals the component's spec and state as JSON, ensuring the
// runtime fields are synced into State first.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState reads JSON from r and restores both the base state and the
// runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}
	c.SetState(c.Component.GetState())
	return nil
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

func (m *middleware) flitParentTaskID(flit *messaging.Flit) string {
	return flit.ID + "_e2e"
}

func (m *middleware) flitTaskID(flit *messaging.Flit) string {
	return flit.ID + "_" + m.Comp.Name()
}

func (m *middleware) startProcessing() (madeProgress bool) {
	for _, port := range m.ports {
		pc := m.portToComplexMapping[port.AsRemote()]

		for i := 0; i < pc.numInputChannel; i++ {
			itemI := port.PeekIncoming()
			if itemI == nil {
				break
			}

			if !pc.pipeline.CanAccept() {
				break
			}

			flit := itemI.(*messaging.Flit)
			pipelineItem := flitPipelineItem{
				taskID: m.flitTaskID(flit),
				flit:   flit,
			}
			pc.pipeline.Accept(pipelineItem)
			port.RetrieveIncoming()

			madeProgress = true

			tracing.StartTask(
				m.flitTaskID(flit),
				m.flitParentTaskID(flit),
				m.Comp, "flit", "flit_inside_sw",
				flit,
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
			flit := pipelineItem.flit
			m.assignFlitOutputBuf(flit)
			routeBuf.Pop()
			forwardBuf.Push(flit)

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

			flit := item.(*messaging.Flit)
			if !flit.OutputBuf.CanPush() {
				break
			}

			flit.OutputBuf.Push(flit)
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

			flit := item.(*messaging.Flit)
			flit.Src = pc.localPort.AsRemote()
			flit.Dst = pc.remotePort

			err := pc.localPort.Send(flit)
			if err == nil {
				madeProgress = true

				sendOutBuf.Pop()

				tracing.EndTask(m.flitTaskID(flit), m.Comp)
			}
		}
	}

	return madeProgress
}

func (m *middleware) assignFlitOutputBuf(flit *messaging.Flit) {
	outPort := m.routingTable.FindPort(flit.Msg.Meta().Dst)
	if outPort == "" {
		panic(fmt.Sprintf("%s: no output port for %s",
			m.Comp.Name(), flit.Msg.Meta().Dst))
	}

	pc := m.portToComplexMapping[outPort]

	flit.OutputBuf = pc.sendOutBuffer
	if flit.OutputBuf == nil {
		panic(fmt.Sprintf("%s: no output buffer for %s",
			m.Comp.Name(), flit.Msg.Meta().Dst))
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
