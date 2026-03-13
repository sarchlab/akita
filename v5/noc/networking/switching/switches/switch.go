// Package switches provides implementations of Switches.
package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/arbitration"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the switch.
type Spec struct{}

// flitMeta is a fully serializable snapshot of a messaging.Flit, including the
// original message metadata so a downstream endpoint can reconstruct the Msg.
type flitMeta struct {
	sim.MsgMeta

	SeqID        int `json:"seq_id"`
	NumFlitInMsg int `json:"num_flit_in_msg"`

	// Original message metadata.
	MsgID           string         `json:"msg_id"`
	MsgSrc          sim.RemotePort `json:"msg_src"`
	MsgDst          sim.RemotePort `json:"msg_dst_meta"`
	MsgRspTo        string         `json:"msg_rsp_to"`
	MsgTrafficClass string         `json:"msg_traffic_class"`
	MsgTrafficBytes int            `json:"msg_traffic_bytes"`
}

// flitMetaFromFlit captures all relevant fields of a *messaging.Flit.
func flitMetaFromFlit(flit *messaging.Flit) flitMeta {
	fm := flitMeta{
		MsgMeta:      flit.MsgMeta,
		SeqID:        flit.SeqID,
		NumFlitInMsg: flit.NumFlitInMsg,
	}
	if flit.Msg != nil {
		m := flit.Msg.Meta()
		fm.MsgID = m.ID
		fm.MsgSrc = m.Src
		fm.MsgDst = m.Dst
		fm.MsgRspTo = m.RspTo
		fm.MsgTrafficClass = m.TrafficClass
		fm.MsgTrafficBytes = m.TrafficBytes
	}
	return fm
}

// toFlit reconstructs a *messaging.Flit from the serialized flitMeta.
func (fm flitMeta) toFlit() *messaging.Flit {
	return &messaging.Flit{
		MsgMeta:      fm.MsgMeta,
		SeqID:        fm.SeqID,
		NumFlitInMsg: fm.NumFlitInMsg,
		Msg: &sim.MsgMeta{
			ID:           fm.MsgID,
			Src:          fm.MsgSrc,
			Dst:          fm.MsgDst,
			RspTo:        fm.MsgRspTo,
			TrafficClass: fm.MsgTrafficClass,
			TrafficBytes: fm.MsgTrafficBytes,
		},
	}
}

// flitPipelineItemState is a serializable flit pipeline item.
type flitPipelineItemState struct {
	TaskID  string         `json:"task_id"`
	Flit    flitMeta       `json:"flit"`
	RouteTo sim.RemotePort `json:"route_to"` // final destination for routing
}

// forwardBufferEntry is a flit waiting to be forwarded, with its assigned output.
type forwardBufferEntry struct {
	Flit         flitMeta `json:"flit"`
	OutputBufIdx int      `json:"output_buf_idx"`
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
	LocalPortName    string               `json:"local_port_name"`
	RemotePort       sim.RemotePort       `json:"remote_port"`
	NumInputChannel  int                  `json:"num_input_channel"`
	NumOutputChannel int                  `json:"num_output_channel"`
	Latency          int                  `json:"latency"`
	PipelineWidth    int                  `json:"pipeline_width"`
	PipelineStages   []pipelineStageState `json:"pipeline_stages"`

	RouteBuffer   stateutil.Buffer[flitPipelineItemState] `json:"route_buffer"`
	ForwardBuffer stateutil.Buffer[forwardBufferEntry]    `json:"forward_buffer"`
	SendOutBuffer stateutil.Buffer[flitMeta]              `json:"send_out_buffer"`
}

// State contains mutable runtime data for the switch.
type State struct {
	PortComplexes []portComplexState `json:"port_complexes"`
}

// --- Thin buffer adapters ---

// forwardBufAdapter wraps a *stateutil.Buffer[forwardBufferEntry] to satisfy
// queueing.Buffer. It delegates to the stateutil.Buffer and resolves
// OutputBufIdx → sendOutBufAdapter on Peek/Pop.
type forwardBufAdapter struct {
	sim.HookableBase
	name    string
	portIdx int
	infra   *switchInfra
}

func (b *forwardBufAdapter) buf() *stateutil.Buffer[forwardBufferEntry] {
	next := b.infra.comp.GetNextState()
	return &next.PortComplexes[b.portIdx].ForwardBuffer
}

func (b *forwardBufAdapter) Name() string  { return b.name }
func (b *forwardBufAdapter) Capacity() int { return b.buf().Capacity() }
func (b *forwardBufAdapter) Size() int     { return b.buf().Size() }
func (b *forwardBufAdapter) CanPush() bool { return b.buf().CanPush() }
func (b *forwardBufAdapter) Clear()        { b.buf().Clear() }

func (b *forwardBufAdapter) Push(e any) {
	flit := e.(*messaging.Flit)
	entry := forwardBufferEntry{
		Flit: flitMetaFromFlit(flit),
	}
	// OutputBuf should be a sendOutBufAdapter; find its index.
	if sob, ok := flit.OutputBuf.(*sendOutBufAdapter); ok {
		entry.OutputBufIdx = sob.portIdx
	}
	b.buf().PushTyped(entry)
}

func (b *forwardBufAdapter) Peek() any {
	buf := b.buf()
	if buf.Size() == 0 {
		return nil
	}
	e := buf.Elements[0]
	flit := e.Flit.toFlit()
	flit.OutputBuf = b.infra.sendOutBufAdapters[e.OutputBufIdx]
	return flit
}

func (b *forwardBufAdapter) Pop() any {
	buf := b.buf()
	if buf.Size() == 0 {
		return nil
	}
	e, _ := buf.PopTyped()
	flit := e.Flit.toFlit()
	flit.OutputBuf = b.infra.sendOutBufAdapters[e.OutputBufIdx]
	return flit
}

// sendOutBufAdapter wraps a *stateutil.Buffer[flitMeta] to satisfy
// queueing.Buffer.
type sendOutBufAdapter struct {
	sim.HookableBase
	name    string
	portIdx int
	infra   *switchInfra
}

func (b *sendOutBufAdapter) buf() *stateutil.Buffer[flitMeta] {
	next := b.infra.comp.GetNextState()
	return &next.PortComplexes[b.portIdx].SendOutBuffer
}

func (b *sendOutBufAdapter) Name() string  { return b.name }
func (b *sendOutBufAdapter) Capacity() int { return b.buf().Capacity() }
func (b *sendOutBufAdapter) Size() int     { return b.buf().Size() }
func (b *sendOutBufAdapter) CanPush() bool { return b.buf().CanPush() }
func (b *sendOutBufAdapter) Clear()        { b.buf().Clear() }

func (b *sendOutBufAdapter) Push(e any) {
	flit := e.(*messaging.Flit)
	b.buf().PushTyped(flitMetaFromFlit(flit))
}

func (b *sendOutBufAdapter) Peek() any {
	buf := b.buf()
	if buf.Size() == 0 {
		return nil
	}
	return buf.Elements[0].toFlit()
}

func (b *sendOutBufAdapter) Pop() any {
	buf := b.buf()
	if buf.Size() == 0 {
		return nil
	}
	fm, _ := buf.PopTyped()
	return fm.toFlit()
}

// --- Free functions for pipeline operations ---

func pipelineCanAccept(pcs portComplexState) bool {
	if pcs.Latency == 0 {
		return pcs.RouteBuffer.CanPush()
	}

	for lane := 0; lane < pcs.PipelineWidth; lane++ {
		if !pipelineSlotOccupied(pcs, lane, 0) {
			return true
		}
	}

	return false
}

func pipelineSlotOccupied(pcs portComplexState, lane, stage int) bool {
	for _, s := range pcs.PipelineStages {
		if s.Lane == lane && s.Stage == stage {
			return true
		}
	}

	return false
}

func pipelineAccept(pcs *portComplexState, item flitPipelineItemState) {
	if pcs.Latency == 0 {
		pcs.RouteBuffer.PushTyped(item)
		return
	}

	for lane := 0; lane < pcs.PipelineWidth; lane++ {
		if !pipelineSlotOccupied(*pcs, lane, 0) {
			pcs.PipelineStages = append(pcs.PipelineStages,
				pipelineStageState{
					Lane:      lane,
					Stage:     0,
					Item:      item,
					CycleLeft: 0,
				})
			return
		}
	}

	panic("pipeline is full, call pipelineCanAccept first")
}

type pipelineAction int

const (
	pipelineActionKeep pipelineAction = iota
	pipelineActionAdvanced
	pipelineActionMoveToBuffer
)

func pipelineTick(pcs *portComplexState) bool {
	if pcs.Latency == 0 {
		return false
	}

	madeProgress := false
	lastStage := pcs.Latency - 1

	actions := make([]pipelineAction, len(pcs.PipelineStages))
	newStages := make([]pipelineStageState, len(pcs.PipelineStages))
	copy(newStages, pcs.PipelineStages)

	for stageNum := lastStage; stageNum >= 0; stageNum-- {
		for i := range newStages {
			if actions[i] != pipelineActionKeep {
				continue
			}

			if newStages[i].Stage != stageNum {
				continue
			}

			act, progress := processStageItem(
				&newStages[i], stageNum, lastStage,
				pcs, newStages, actions,
			)
			actions[i] = act
			madeProgress = madeProgress || progress
		}
	}

	remaining := make([]pipelineStageState, 0, len(newStages))

	for i, a := range actions {
		if a != pipelineActionMoveToBuffer {
			remaining = append(remaining, newStages[i])
		}
	}

	pcs.PipelineStages = remaining

	return madeProgress
}

func processStageItem(
	s *pipelineStageState,
	stageNum, lastStage int,
	pcs *portComplexState,
	newStages []pipelineStageState,
	actions []pipelineAction,
) (pipelineAction, bool) {
	if s.CycleLeft > 0 {
		s.CycleLeft--
		return pipelineActionKeep, true
	}

	if stageNum == lastStage {
		return tryMoveToRouteBuffer(s, pcs)
	}

	return tryAdvanceStage(s, stageNum, newStages, actions)
}

func tryMoveToRouteBuffer(
	s *pipelineStageState,
	pcs *portComplexState,
) (pipelineAction, bool) {
	if pcs.RouteBuffer.CanPush() {
		pcs.RouteBuffer.PushTyped(s.Item)
		return pipelineActionMoveToBuffer, true
	}

	return pipelineActionKeep, false
}

func tryAdvanceStage(
	s *pipelineStageState,
	stageNum int,
	newStages []pipelineStageState,
	actions []pipelineAction,
) (pipelineAction, bool) {
	nextStageNum := stageNum + 1

	if isNextStageOccupied(s.Lane, nextStageNum, newStages, actions) {
		return pipelineActionKeep, false
	}

	s.Stage = nextStageNum
	s.CycleLeft = 0

	return pipelineActionAdvanced, true
}

func isNextStageOccupied(
	lane, stage int,
	stages []pipelineStageState,
	actions []pipelineAction,
) bool {
	for j := range stages {
		if actions[j] != pipelineActionKeep {
			continue
		}

		if stages[j].Lane == lane && stages[j].Stage == stage {
			return true
		}
	}

	return false
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

// --- Shared infrastructure ---

// switchInfra holds state shared by both middlewares.
type switchInfra struct {
	comp      *modeling.Component[Spec, State]
	ports     []sim.Port
	portIndex map[sim.RemotePort]int // remotePort → index in State.PortComplexes

	// Thin buffer adapters (created once, access buffers dynamically)
	forwardBufAdapters []*forwardBufAdapter
	sendOutBufAdapters []*sendOutBufAdapter
}

// addPort registers a port complex with the shared infrastructure.
func (inf *switchInfra) addPort(
	port sim.Port,
	remotePort sim.RemotePort,
	pcs portComplexState,
	arbiter arbitration.Arbiter,
) {
	idx := len(inf.ports)
	inf.ports = append(inf.ports, port)
	inf.portIndex[remotePort] = idx

	// Also map the local port's RemotePort so assignFlitOutputBuf can find it
	inf.portIndex[port.AsRemote()] = idx

	// Initialize stateutil.Buffer fields
	pcs.RouteBuffer = stateutil.Buffer[flitPipelineItemState]{
		BufferName: pcs.LocalPortName + "RouteBuf",
		Cap:        pcs.NumInputChannel,
	}
	pcs.ForwardBuffer = stateutil.Buffer[forwardBufferEntry]{
		BufferName: pcs.LocalPortName + "FwdBuf",
		Cap:        pcs.NumInputChannel,
	}
	pcs.SendOutBuffer = stateutil.Buffer[flitMeta]{
		BufferName: pcs.LocalPortName + "SendBuf",
		Cap:        pcs.NumOutputChannel,
	}

	// Initialize state in both current and next buffers
	next := inf.comp.GetNextState()
	next.PortComplexes = append(next.PortComplexes, pcs)
	inf.comp.SetState(*next)

	// Create thin wrapper adapters
	sendAdapter := &sendOutBufAdapter{
		name:    pcs.LocalPortName + "SendBuf",
		portIdx: idx,
		infra:   inf,
	}
	fwdAdapter := &forwardBufAdapter{
		name:    pcs.LocalPortName + "FwdBuf",
		portIdx: idx,
		infra:   inf,
	}

	inf.sendOutBufAdapters = append(inf.sendOutBufAdapters, sendAdapter)
	inf.forwardBufAdapters = append(inf.forwardBufAdapters, fwdAdapter)

	arbiter.AddBuffer(fwdAdapter)
}

// --- routeForwardSendMW ---

type routeForwardSendMW struct {
	*switchInfra
	routingTable routing.Table
	arbiter      arbitration.Arbiter
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

			pcs.ForwardBuffer.PushTyped(forwardBufferEntry{
				Flit:         item.Flit,
				OutputBufIdx: outputBufIdx,
			})

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
			flit := fm.toFlit()
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
	*switchInfra
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

			if !pipelineCanAccept(*nextPcs) {
				break
			}

			flit := itemI.(*messaging.Flit)
			item := flitPipelineItemState{
				TaskID:  m.flitTaskID(flit),
				Flit:    flitMetaFromFlit(flit),
				RouteTo: flit.Msg.Meta().Dst,
			}
			pipelineAccept(nextPcs, item)
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
		madeProgress = pipelineTick(&next.PortComplexes[i]) || madeProgress
	}

	return madeProgress
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
	rfsMW.addPort(a.localPort, a.remotePort.AsRemote(), pcs, rfsMW.arbiter)
}
