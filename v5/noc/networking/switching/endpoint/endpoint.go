// Package endpoint provides endpoint
package endpoint

import (
	"fmt"
	"io"
	"math"
	"reflect"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the endpoint.
type Spec struct {
	NumInputChannels  int     `json:"num_input_channels"`
	NumOutputChannels int     `json:"num_output_channels"`
	FlitByteSize      int     `json:"flit_byte_size"`
	EncodingOverhead  float64 `json:"encoding_overhead"`
}

// msgRef is a serializable representation of a sim.Msg.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
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
	MsgOutBuf      []msgRef             `json:"msg_out_buf"`
	FlitsToSend    []flitState          `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []msgRef             `json:"assembled_msgs"`
}

type msgToAssemble struct {
	msg             sim.Msg
	numFlitRequired int
	numFlitArrived  int
}

// Comp is an akita component(Endpoint) that delegates sending and receiving
// actions of a few ports.
type Comp struct {
	*modeling.Component[Spec, State]

	NetworkPort      sim.Port
	DevicePorts      []sim.Port
	DefaultSwitchDst sim.RemotePort

	msgOutBuf   []sim.Msg
	flitsToSend []*messaging.Flit

	assemblingMsgs []*msgToAssemble
	assembledMsgs  []sim.Msg
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

// snapshotState converts runtime mutable data into a serializable State.
func (c *Comp) snapshotState() State {
	s := State{}

	s.MsgOutBuf = make([]msgRef, len(c.msgOutBuf))
	for i, msg := range c.msgOutBuf {
		s.MsgOutBuf[i] = msgRefFromMsg(msg)
	}

	s.FlitsToSend = make([]flitState, len(c.flitsToSend))
	for i, flit := range c.flitsToSend {
		s.FlitsToSend[i] = flitStateFromFlit(flit)
	}

	s.AssemblingMsgs = make([]assemblingMsgState, len(c.assemblingMsgs))
	for i, a := range c.assemblingMsgs {
		meta := a.msg.Meta()
		s.AssemblingMsgs[i] = assemblingMsgState{
			MsgID:           meta.ID,
			Src:             meta.Src,
			Dst:             meta.Dst,
			RspTo:           meta.RspTo,
			TrafficClass:    meta.TrafficClass,
			TrafficBytes:    meta.TrafficBytes,
			NumFlitRequired: a.numFlitRequired,
			NumFlitArrived:  a.numFlitArrived,
		}
	}

	s.AssembledMsgs = make([]msgRef, len(c.assembledMsgs))
	for i, msg := range c.assembledMsgs {
		s.AssembledMsgs[i] = msgRefFromMsg(msg)
	}

	return s
}

// restoreFromState restores runtime mutable data from a serializable State.
func (c *Comp) restoreFromState(s State) {
	c.msgOutBuf = make([]sim.Msg, len(s.MsgOutBuf))
	for i, ref := range s.MsgOutBuf {
		c.msgOutBuf[i] = msgFromRef(ref)
	}

	c.flitsToSend = make([]*messaging.Flit, len(s.FlitsToSend))
	for i, fs := range s.FlitsToSend {
		originalMsg := &sim.MsgMeta{
			ID: fs.OriginalMsgID,
		}
		c.flitsToSend[i] = &messaging.Flit{
			MsgMeta: sim.MsgMeta{
				ID:  fs.ID,
				Src: fs.Src,
				Dst: fs.Dst,
			},
			SeqID:        fs.SeqID,
			NumFlitInMsg: fs.NumFlitInMsg,
			Msg:          originalMsg,
		}
	}

	c.assemblingMsgs = make([]*msgToAssemble, len(s.AssemblingMsgs))
	for i, as := range s.AssemblingMsgs {
		c.assemblingMsgs[i] = &msgToAssemble{
			msg: &sim.MsgMeta{
				ID:           as.MsgID,
				Src:          as.Src,
				Dst:          as.Dst,
				RspTo:        as.RspTo,
				TrafficClass: as.TrafficClass,
				TrafficBytes: as.TrafficBytes,
			},
			numFlitRequired: as.NumFlitRequired,
			numFlitArrived:  as.NumFlitArrived,
		}
	}

	c.assembledMsgs = make([]sim.Msg, len(s.AssembledMsgs))
	for i, ref := range s.AssembledMsgs {
		c.assembledMsgs[i] = msgFromRef(ref)
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

// SyncState copies mutable runtime data into the State struct.
// Deprecated: Use GetState() instead.
func (c *Comp) SyncState() {
	c.GetState()
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

func msgFromRef(ref msgRef) sim.Msg {
	return &sim.MsgMeta{
		ID:           ref.ID,
		Src:          ref.Src,
		Dst:          ref.Dst,
		RspTo:        ref.RspTo,
		TrafficClass: ref.TrafficClass,
		TrafficBytes: ref.TrafficBytes,
	}
}

func flitStateFromFlit(flit *messaging.Flit) flitState {
	return flitState{
		ID:            flit.ID,
		Src:           flit.Src,
		Dst:           flit.Dst,
		SeqID:         flit.SeqID,
		NumFlitInMsg:  flit.NumFlitInMsg,
		OriginalMsgID: flit.Msg.Meta().ID,
	}
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

func (m *middleware) flitTaskID(flit *messaging.Flit) string {
	return fmt.Sprintf("%s_e2e", flit.ID)
}

func (m *middleware) sendFlitOut() bool {
	madeProgress := false

	for i := 0; i < m.Comp.GetSpec().NumOutputChannels; i++ {
		if len(m.flitsToSend) == 0 {
			return madeProgress
		}

		flit := m.flitsToSend[0]
		err := m.NetworkPort.Send(flit)

		if err == nil {
			m.flitsToSend = m.flitsToSend[1:]

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

		for _, flit := range flits {
			m.logFlitE2ETask(flit, false)
		}

		madeProgress = true
	}
}

func (m *middleware) recv() bool {
	madeProgress := false

	for i := 0; i < m.Comp.GetSpec().NumInputChannels; i++ {
		receivedI := m.NetworkPort.PeekIncoming()
		if receivedI == nil {
			return madeProgress
		}

		flit := receivedI.(*messaging.Flit)
		msg := flit.Msg

		var assembling *msgToAssemble
		for _, a := range m.assemblingMsgs {
			if a.msg.Meta().ID == msg.Meta().ID {
				assembling = a
				break
			}
		}

		if assembling == nil {
			assembling = &msgToAssemble{
				msg:             msg,
				numFlitRequired: flit.NumFlitInMsg,
				numFlitArrived:  0,
			}
			m.assemblingMsgs = append(m.assemblingMsgs, assembling)
		}

		assembling.numFlitArrived++

		m.NetworkPort.RetrieveIncoming()

		m.logFlitE2ETask(flit, true)

		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) assemble() bool {
	madeProgress := false

	remaining := m.assemblingMsgs[:0]
	for _, assembling := range m.assemblingMsgs {
		if assembling.numFlitArrived < assembling.numFlitRequired {
			remaining = append(remaining, assembling)
			continue
		}

		m.assembledMsgs = append(m.assembledMsgs, assembling.msg)
		madeProgress = true
	}

	m.assemblingMsgs = remaining

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
		tracing.EndTask(m.msgTaskID(meta.ID), m.Comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.ID+"_req_out",
			m.Comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *middleware) logMsgRsp(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m.Comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.RspTo+"_req_out",
			m.Comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *middleware) msgToFlits(msg sim.Msg) []*messaging.Flit {
	numFlit := 1
	meta := msg.Meta()

	if meta.TrafficBytes > 0 {
		trafficByte := meta.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * m.Comp.GetSpec().EncodingOverhead))
		numFlit = (trafficByte-1)/m.Comp.GetSpec().FlitByteSize + 1
	}

	flits := make([]*messaging.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", i, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.Src = m.NetworkPort.AsRemote()
		flit.Dst = m.DefaultSwitchDst
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.SeqID = i
		flit.NumFlitInMsg = numFlit
		flit.Msg = msg
		flits[i] = flit
	}

	return flits
}
