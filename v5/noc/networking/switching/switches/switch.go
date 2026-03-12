// Package switches provides implementations of Switches.
package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/arbitration"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type flitPipelineItem struct {
	taskID string
	flit   *messaging.Flit
}

func (f flitPipelineItem) TaskID() string {
	return f.taskID
}

// Spec contains immutable configuration for the switch.
type Spec struct{}

// flitPipelineItemState is a serializable flit pipeline item.
type flitPipelineItemState struct {
	TaskID string         `json:"task_id"`
	Flit   sim.MsgMeta    `json:"flit"`
	MsgDst sim.RemotePort `json:"msg_dst"` // final destination for routing
}

// forwardBufferEntry is a flit waiting to be forwarded, with its assigned output.
type forwardBufferEntry struct {
	Flit         sim.MsgMeta `json:"flit"`
	OutputBufIdx int         `json:"output_buf_idx"`
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
	LocalPortName    string                  `json:"local_port_name"`
	RemotePort       sim.RemotePort          `json:"remote_port"`
	NumInputChannel  int                     `json:"num_input_channel"`
	NumOutputChannel int                     `json:"num_output_channel"`
	Latency          int                     `json:"latency"`
	PipelineWidth    int                     `json:"pipeline_width"`
	PipelineStages   []pipelineStageState    `json:"pipeline_stages"`
	RouteBuffer      []flitPipelineItemState `json:"route_buffer"`
	ForwardBuffer    []forwardBufferEntry    `json:"forward_buffer"`
	SendOutBuffer    []sim.MsgMeta           `json:"send_out_buffer"`
}

// State contains mutable runtime data for the switch.
type State struct {
	PortComplexes []portComplexState `json:"port_complexes"`
}

// --- Thin buffer adapters ---

// stateFlitBuffer wraps a *[]flitPipelineItemState to satisfy queueing.Buffer.
// Used for routeBuffer.
type stateFlitBuffer struct {
	sim.HookableBase
	name     string
	items    *[]flitPipelineItemState
	capacity int
}

func (b *stateFlitBuffer) Name() string    { return b.name }
func (b *stateFlitBuffer) Capacity() int   { return b.capacity }
func (b *stateFlitBuffer) Size() int       { return len(*b.items) }
func (b *stateFlitBuffer) CanPush() bool   { return len(*b.items) < b.capacity }
func (b *stateFlitBuffer) Clear()          { *b.items = nil }

func (b *stateFlitBuffer) Push(e interface{}) {
	item := e.(flitPipelineItem)
	*b.items = append(*b.items, flitPipelineItemState{
		TaskID: item.taskID,
		Flit:   item.flit.MsgMeta,
	})
}

func (b *stateFlitBuffer) Peek() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	s := (*b.items)[0]
	return flitPipelineItem{
		taskID: s.TaskID,
		flit:   &messaging.Flit{MsgMeta: s.Flit},
	}
}

func (b *stateFlitBuffer) Pop() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	s := (*b.items)[0]
	*b.items = (*b.items)[1:]
	return flitPipelineItem{
		taskID: s.TaskID,
		flit:   &messaging.Flit{MsgMeta: s.Flit},
	}
}

// stateForwardBuffer wraps a *[]forwardBufferEntry to satisfy queueing.Buffer.
// Used for forwardBuffer. Peek/Pop reconstruct a *messaging.Flit with OutputBuf set.
type stateForwardBuffer struct {
	sim.HookableBase
	name       string
	items      *[]forwardBufferEntry
	capacity   int
	mw         *middleware // needed to resolve OutputBufIdx → adapter
}

func (b *stateForwardBuffer) Name() string    { return b.name }
func (b *stateForwardBuffer) Capacity() int   { return b.capacity }
func (b *stateForwardBuffer) Size() int       { return len(*b.items) }
func (b *stateForwardBuffer) CanPush() bool   { return len(*b.items) < b.capacity }
func (b *stateForwardBuffer) Clear()          { *b.items = nil }

func (b *stateForwardBuffer) Push(e interface{}) {
	flit := e.(*messaging.Flit)
	entry := forwardBufferEntry{
		Flit: flit.MsgMeta,
	}
	// OutputBuf should be a stateSendOutBuffer; find its index.
	if sob, ok := flit.OutputBuf.(*stateSendOutBuffer); ok {
		entry.OutputBufIdx = sob.portIdx
	}
	*b.items = append(*b.items, entry)
}

func (b *stateForwardBuffer) Peek() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	e := (*b.items)[0]
	flit := &messaging.Flit{MsgMeta: e.Flit}
	flit.OutputBuf = b.mw.sendOutBufAdapters[e.OutputBufIdx]
	return flit
}

func (b *stateForwardBuffer) Pop() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	e := (*b.items)[0]
	*b.items = (*b.items)[1:]
	flit := &messaging.Flit{MsgMeta: e.Flit}
	flit.OutputBuf = b.mw.sendOutBufAdapters[e.OutputBufIdx]
	return flit
}

// stateSendOutBuffer wraps a *[]sim.MsgMeta to satisfy queueing.Buffer.
// Used for sendOutBuffer.
type stateSendOutBuffer struct {
	sim.HookableBase
	name     string
	items    *[]sim.MsgMeta
	capacity int
	portIdx  int // index of this port in middleware.ports
}

func (b *stateSendOutBuffer) Name() string    { return b.name }
func (b *stateSendOutBuffer) Capacity() int   { return b.capacity }
func (b *stateSendOutBuffer) Size() int       { return len(*b.items) }
func (b *stateSendOutBuffer) CanPush() bool   { return len(*b.items) < b.capacity }
func (b *stateSendOutBuffer) Clear()          { *b.items = nil }

func (b *stateSendOutBuffer) Push(e interface{}) {
	flit := e.(*messaging.Flit)
	*b.items = append(*b.items, flit.MsgMeta)
}

func (b *stateSendOutBuffer) Peek() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	return &messaging.Flit{MsgMeta: (*b.items)[0]}
}

func (b *stateSendOutBuffer) Pop() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	meta := (*b.items)[0]
	*b.items = (*b.items)[1:]
	return &messaging.Flit{MsgMeta: meta}
}

// --- Free functions for pipeline operations ---

func pipelineCanAccept(pcs portComplexState) bool {
	if pcs.Latency == 0 {
		return len(pcs.RouteBuffer) < pcs.NumInputChannel
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
		pcs.RouteBuffer = append(pcs.RouteBuffer, item)
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
	if len(pcs.RouteBuffer) < pcs.NumInputChannel {
		pcs.RouteBuffer = append(pcs.RouteBuffer, s.Item)
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

	mw *middleware // internal reference for port addition and delegation
}

// GetRoutingTable returns the routine table used by the switch.
func (c *Comp) GetRoutingTable() routing.Table {
	return c.mw.routingTable
}

// --- Middleware ---

type middleware struct {
	comp         *modeling.Component[Spec, State]
	routingTable routing.Table
	arbiter      arbitration.Arbiter

	ports     []sim.Port
	portIndex map[sim.RemotePort]int // remotePort → index in State.PortComplexes

	// Thin buffer adapters (created once, pointers updated per-tick)
	forwardBufAdapters []*stateForwardBuffer
	sendOutBufAdapters []*stateSendOutBuffer
	routeBufAdapters   []*stateFlitBuffer
}

// NamedHookable delegation methods.

func (m *middleware) Name() string {
	return m.comp.Name()
}

func (m *middleware) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *middleware) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *middleware) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *middleware) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

// addPort registers a port complex with the middleware.
func (m *middleware) addPort(port sim.Port, remotePort sim.RemotePort, pcs portComplexState) {
	idx := len(m.ports)
	m.ports = append(m.ports, port)
	m.portIndex[remotePort] = idx

	// Also map the local port's RemotePort so assignFlitOutputBuf can find it
	m.portIndex[port.AsRemote()] = idx

	// Initialize state in both current and next buffers
	next := m.comp.GetNextState()
	next.PortComplexes = append(next.PortComplexes, pcs)
	m.comp.SetState(*next)

	// Create adapters with dummy slice pointers (will be updated in updateAdapterPointers)
	sendAdapter := &stateSendOutBuffer{name: pcs.LocalPortName + "SendBuf", capacity: pcs.NumOutputChannel, portIdx: idx}
	fwdAdapter := &stateForwardBuffer{name: pcs.LocalPortName + "FwdBuf", capacity: pcs.NumInputChannel, mw: m}
	routeAdapter := &stateFlitBuffer{name: pcs.LocalPortName + "RouteBuf", capacity: pcs.NumInputChannel}

	m.sendOutBufAdapters = append(m.sendOutBufAdapters, sendAdapter)
	m.forwardBufAdapters = append(m.forwardBufAdapters, fwdAdapter)
	m.routeBufAdapters = append(m.routeBufAdapters, routeAdapter)

	// Point adapters at next state data (will be updated per-tick)
	next = m.comp.GetNextState()
	fwdAdapter.items = &next.PortComplexes[idx].ForwardBuffer
	sendAdapter.items = &next.PortComplexes[idx].SendOutBuffer
	routeAdapter.items = &next.PortComplexes[idx].RouteBuffer

	m.arbiter.AddBuffer(fwdAdapter)
}

func (m *middleware) updateAdapterPointers() {
	next := m.comp.GetNextState()
	for i := range m.ports {
		m.forwardBufAdapters[i].items = &next.PortComplexes[i].ForwardBuffer
		m.sendOutBufAdapters[i].items = &next.PortComplexes[i].SendOutBuffer
		m.routeBufAdapters[i].items = &next.PortComplexes[i].RouteBuffer
	}
}

// Tick updates the Switch's state.
func (m *middleware) Tick() bool {
	m.updateAdapterPointers()

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
	return flit.ID + "_" + m.comp.Name()
}

func (m *middleware) startProcessing() (madeProgress bool) {
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
				TaskID: m.flitTaskID(flit),
				Flit:   flit.MsgMeta,
				MsgDst: flit.Msg.Meta().Dst,
			}
			pipelineAccept(nextPcs, item)
			port.RetrieveIncoming()

			madeProgress = true

			tracing.StartTask(
				m.flitTaskID(flit),
				m.flitParentTaskID(flit),
				m, "flit", "flit_inside_sw",
				flit,
			)
		}
	}

	return madeProgress
}

func (m *middleware) movePipeline() (madeProgress bool) {
	next := m.comp.GetNextState()

	for i := range m.ports {
		madeProgress = pipelineTick(&next.PortComplexes[i]) || madeProgress
	}

	return madeProgress
}

func (m *middleware) route() (madeProgress bool) {
	next := m.comp.GetNextState()

	for i := range m.ports {
		pcs := &next.PortComplexes[i]

		for j := 0; j < pcs.NumInputChannel; j++ {
			if len(pcs.RouteBuffer) == 0 {
				break
			}

			if len(pcs.ForwardBuffer) >= pcs.NumInputChannel {
				break
			}

			item := pcs.RouteBuffer[0]
			outputBufIdx := m.resolveOutputBufIdx(item.MsgDst)

			pcs.RouteBuffer = pcs.RouteBuffer[1:]
			pcs.ForwardBuffer = append(pcs.ForwardBuffer, forwardBufferEntry{
				Flit:         item.Flit,
				OutputBufIdx: outputBufIdx,
			})

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) resolveOutputBufIdx(msgDst sim.RemotePort) int {
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
	cur := m.comp.GetState()

	for i, port := range m.ports {
		curPcs := &cur.PortComplexes[i]
		numSent := 0

		for j := 0; j < curPcs.NumOutputChannel; j++ {
			if numSent >= len(curPcs.SendOutBuffer) {
				break
			}

			meta := curPcs.SendOutBuffer[numSent]
			flit := &messaging.Flit{MsgMeta: meta}
			flit.Src = port.AsRemote()
			flit.Dst = curPcs.RemotePort

			err := port.Send(flit)
			if err == nil {
				madeProgress = true
				numSent++

				tracing.EndTask(m.flitTaskID(flit), m)
			}
		}

		if numSent > 0 {
			next := m.comp.GetNextState()
			nextPcs := &next.PortComplexes[i]
			nextPcs.SendOutBuffer = nextPcs.SendOutBuffer[numSent:]
		}
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
	a.sw.mw.addPort(a.localPort, a.remotePort.AsRemote(), pcs)
}
