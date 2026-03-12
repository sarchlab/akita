// Package endpoint provides endpoint
package endpoint

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the endpoint.
type Spec struct {
	NumInputChannels  int            `json:"num_input_channels"`
	NumOutputChannels int            `json:"num_output_channels"`
	FlitByteSize      int            `json:"flit_byte_size"`
	EncodingOverhead  float64        `json:"encoding_overhead"`
	DefaultSwitchDst  sim.RemotePort `json:"default_switch_dst"`
}

// flitState is a serializable representation of a *messaging.Flit.
type flitState struct {
	ID            string         `json:"id"`
	Src           sim.RemotePort `json:"src"`
	Dst           sim.RemotePort `json:"dst"`
	SeqID         int            `json:"seq_id"`
	NumFlitInMsg  int            `json:"num_flit_in_msg"`
	OriginalMsgID string         `json:"original_msg_id"`
}

// assemblingMsgState is a serializable representation of a message being
// assembled from flits.
type assemblingMsgState struct {
	MsgID           string         `json:"msg_id"`
	Src             sim.RemotePort `json:"src"`
	Dst             sim.RemotePort `json:"dst"`
	RspTo           string         `json:"rsp_to"`
	TrafficClass    string         `json:"traffic_class"`
	TrafficBytes    int            `json:"traffic_bytes"`
	NumFlitRequired int            `json:"num_flit_required"`
	NumFlitArrived  int            `json:"num_flit_arrived"`
}

// State contains mutable runtime data for the endpoint.
type State struct {
	MsgOutBuf      []sim.MsgMeta        `json:"msg_out_buf"`
	FlitsToSend    []flitState          `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []sim.MsgMeta        `json:"assembled_msgs"`
}

// Comp is an akita component(Endpoint) that delegates sending and receiving
// actions of a few ports.
type Comp struct {
	*modeling.Component[Spec, State]
}

// mw returns the middleware from the component's middleware list.
func (c *Comp) mw() *middleware {
	return c.Middlewares()[0].(*middleware)
}

// NetworkPort returns the network port of the endpoint.
func (c *Comp) NetworkPort() sim.Port {
	return c.mw().networkPort
}

// SetNetworkPort sets the network port of the endpoint.
func (c *Comp) SetNetworkPort(p sim.Port) {
	c.mw().networkPort = p
}

// DefaultSwitchDst returns the default switch destination.
func (c *Comp) DefaultSwitchDst() sim.RemotePort {
	return c.mw().defaultSwitchDst
}

// SetDefaultSwitchDst sets the default switch destination.
func (c *Comp) SetDefaultSwitchDst(dst sim.RemotePort) {
	c.mw().defaultSwitchDst = dst
}

// PlugIn connects a port to the endpoint.
func (c *Comp) PlugIn(port sim.Port) {
	port.SetConnection(c)
	c.mw().devicePorts = append(c.mw().devicePorts, port)
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

func flitFromFlitState(fs flitState) *messaging.Flit {
	return &messaging.Flit{
		MsgMeta: sim.MsgMeta{
			ID:  fs.ID,
			Src: fs.Src,
			Dst: fs.Dst,
		},
		SeqID:        fs.SeqID,
		NumFlitInMsg: fs.NumFlitInMsg,
		Msg: &sim.MsgMeta{
			ID: fs.OriginalMsgID,
		},
	}
}

// msgMetaToFlitStates converts a MsgMeta into a slice of flitState entries.
func msgMetaToFlitStates(
	meta sim.MsgMeta,
	spec Spec,
	networkPortRemote sim.RemotePort,
	defaultSwitchDst sim.RemotePort,
) []flitState {
	numFlit := 1
	if meta.TrafficBytes > 0 {
		trafficByte := meta.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * spec.EncodingOverhead))
		numFlit = (trafficByte-1)/spec.FlitByteSize + 1
	}

	flits := make([]flitState, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = flitState{
			ID:            fmt.Sprintf("flit-%d-msg-%s-%s", i, meta.ID, sim.GetIDGenerator().Generate()),
			Src:           networkPortRemote,
			Dst:           defaultSwitchDst,
			SeqID:         i,
			NumFlitInMsg:  numFlit,
			OriginalMsgID: meta.ID,
		}
	}

	return flits
}

type middleware struct {
	comp           *modeling.Component[Spec, State]
	devicePorts    []sim.Port
	networkPort    sim.Port
	defaultSwitchDst sim.RemotePort
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

// Tick update the endpoint state.
func (m *middleware) Tick() bool {
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

func (m *middleware) flitTaskID(flitID string) string {
	return fmt.Sprintf("%s_e2e", flitID)
}

func (m *middleware) sendFlitOut() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()

	numSent := 0

	for i := 0; i < spec.NumOutputChannels; i++ {
		if numSent >= len(cur.FlitsToSend) {
			return madeProgress
		}

		fs := cur.FlitsToSend[numSent]
		flit := flitFromFlitState(fs)

		err := m.networkPort.Send(flit)
		if err == nil {
			numSent++
			madeProgress = true
		}
	}

	if numSent > 0 {
		next := m.comp.GetNextState()
		next.FlitsToSend = next.FlitsToSend[numSent:]

		if len(next.FlitsToSend) == 0 {
			for _, p := range m.devicePorts {
				p.NotifyAvailable()
			}
		}
	}

	return madeProgress
}

func (m *middleware) prepareMsg() bool {
	madeProgress := false

	for i := 0; i < len(m.devicePorts); i++ {
		port := m.devicePorts[i]
		if port.PeekOutgoing() == nil {
			continue
		}

		msg := port.RetrieveOutgoing()
		next := m.comp.GetNextState()
		next.MsgOutBuf = append(next.MsgOutBuf, *msg.Meta())

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) prepareFlits() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	networkPortRemote := m.networkPort.AsRemote()

	for {
		if len(next.MsgOutBuf) == 0 {
			return madeProgress
		}

		meta := next.MsgOutBuf[0]
		next.MsgOutBuf = next.MsgOutBuf[1:]
		flitStates := msgMetaToFlitStates(meta, spec, networkPortRemote, m.defaultSwitchDst)
		next.FlitsToSend = append(next.FlitsToSend, flitStates...)

		for _, fs := range flitStates {
			m.logFlitE2ETaskFromState(fs, false, &meta)
		}

		madeProgress = true
	}
}

func (m *middleware) recv() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	for i := 0; i < spec.NumInputChannels; i++ {
		receivedI := m.networkPort.PeekIncoming()
		if receivedI == nil {
			return madeProgress
		}

		flit := receivedI.(*messaging.Flit)
		msg := flit.Msg

		var assemblingIdx int = -1
		for j, a := range next.AssemblingMsgs {
			if a.MsgID == msg.Meta().ID {
				assemblingIdx = j
				break
			}
		}

		if assemblingIdx < 0 {
			next.AssemblingMsgs = append(next.AssemblingMsgs, assemblingMsgState{
				MsgID:           msg.Meta().ID,
				Src:             msg.Meta().Src,
				Dst:             msg.Meta().Dst,
				RspTo:           msg.Meta().RspTo,
				TrafficClass:    msg.Meta().TrafficClass,
				TrafficBytes:    msg.Meta().TrafficBytes,
				NumFlitRequired: flit.NumFlitInMsg,
				NumFlitArrived:  1,
			})
		} else {
			next.AssemblingMsgs[assemblingIdx].NumFlitArrived++
		}

		m.networkPort.RetrieveIncoming()

		m.logFlitE2ETaskFromFlit(flit, true)

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) assemble() bool {
	madeProgress := false
	cur := m.comp.GetState()
	next := m.comp.GetNextState()

	remaining := make([]assemblingMsgState, 0, len(cur.AssemblingMsgs))

	for _, a := range cur.AssemblingMsgs {
		if a.NumFlitArrived < a.NumFlitRequired {
			remaining = append(remaining, a)
			continue
		}

		assembled := sim.MsgMeta{
			ID:           a.MsgID,
			Src:          a.Src,
			Dst:          a.Dst,
			RspTo:        a.RspTo,
			TrafficClass: a.TrafficClass,
			TrafficBytes: a.TrafficBytes,
		}
		next.AssembledMsgs = append(next.AssembledMsgs, assembled)
		madeProgress = true
	}

	next.AssemblingMsgs = remaining

	return madeProgress
}

func (m *middleware) tryDeliver() bool {
	madeProgress := false
	cur := m.comp.GetState()

	numDelivered := 0

	for i := 0; i < len(cur.AssembledMsgs); i++ {
		meta := cur.AssembledMsgs[i]
		dst := meta.Dst

		var dstPort sim.Port

		for _, port := range m.devicePorts {
			if port.AsRemote() == dst {
				dstPort = port
				break
			}
		}

		if dstPort == nil {
			panic(fmt.Sprintf("no dst port found for %s", dst))
		}

		msg := &sim.MsgMeta{
			ID:           meta.ID,
			Src:          meta.Src,
			Dst:          meta.Dst,
			RspTo:        meta.RspTo,
			TrafficClass: meta.TrafficClass,
			TrafficBytes: meta.TrafficBytes,
		}

		err := dstPort.Deliver(msg)
		if err != nil {
			break
		}

		m.logMsgE2ETask(msg, true)

		numDelivered++
		madeProgress = true
	}

	if numDelivered > 0 {
		next := m.comp.GetNextState()
		next.AssembledMsgs = next.AssembledMsgs[numDelivered:]
	}

	return madeProgress
}

func (m *middleware) logFlitE2ETaskFromState(
	fs flitState, isEnd bool, meta *sim.MsgMeta,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.flitTaskID(fs.ID), m)
		return
	}

	flit := flitFromFlitState(fs)
	flit.Msg = meta

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(fs.ID), m.msgTaskID(meta.ID),
		m, "flit_e2e", "flit_e2e", m.comp.Name()+".FlitBuf", flit,
	)
}

func (m *middleware) logFlitE2ETaskFromFlit(
	flit *messaging.Flit, isEnd bool,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.flitTaskID(flit.ID), m)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(flit.ID), m.msgTaskID(flit.Msg.Meta().ID),
		m, "flit_e2e", "flit_e2e", m.comp.Name()+".FlitBuf", flit,
	)
}

func (m *middleware) logMsgE2ETask(msg sim.Msg, isEnd bool) {
	if m.comp.NumHooks() == 0 {
		return
	}

	meta := msg.Meta()

	if meta.IsRsp() {
		m.logMsgRsp(isEnd, msg)
		return
	}

	m.logMsgReq(isEnd, msg)
}

func (m *middleware) logMsgReq(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.ID+"_req_out",
			m, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *middleware) logMsgRsp(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.RspTo+"_req_out",
			m, "msg_e2e", "msg_e2e", msg,
		)
	}
}
