// Package endpoint provides endpoint
package endpoint

import (
	"fmt"
	"io"
	"math"

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

// msgRef is a serializable representation of a *sim.GenericMsg.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// flitState is a serializable representation of a flit *sim.GenericMsg.
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
	msg             *sim.GenericMsg
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

	msgOutBuf   []*sim.GenericMsg
	flitsToSend []*sim.GenericMsg

	assemblingMsgs []*msgToAssemble
	assembledMsgs  []*sim.GenericMsg
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
		s.FlitsToSend[i] = flitStateFromMsg(flit)
	}

	s.AssemblingMsgs = make([]assemblingMsgState, len(c.assemblingMsgs))
	for i, a := range c.assemblingMsgs {
		s.AssemblingMsgs[i] = assemblingMsgState{
			MsgID:           a.msg.ID,
			Src:             a.msg.Src,
			Dst:             a.msg.Dst,
			RspTo:           a.msg.RspTo,
			TrafficClass:    a.msg.TrafficClass,
			TrafficBytes:    a.msg.TrafficBytes,
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
	c.msgOutBuf = make([]*sim.GenericMsg, len(s.MsgOutBuf))
	for i, ref := range s.MsgOutBuf {
		c.msgOutBuf[i] = msgFromRef(ref)
	}

	c.flitsToSend = make([]*sim.GenericMsg, len(s.FlitsToSend))
	for i, fs := range s.FlitsToSend {
		originalMsg := &sim.GenericMsg{
			MsgMeta: sim.MsgMeta{
				ID: fs.OriginalMsgID,
			},
		}
		c.flitsToSend[i] = &sim.GenericMsg{
			MsgMeta: sim.MsgMeta{
				ID:  fs.ID,
				Src: fs.Src,
				Dst: fs.Dst,
			},
			Payload: &messaging.FlitPayload{
				SeqID:        fs.SeqID,
				NumFlitInMsg: fs.NumFlitInMsg,
				Msg:          originalMsg,
			},
		}
	}

	c.assemblingMsgs = make([]*msgToAssemble, len(s.AssemblingMsgs))
	for i, as := range s.AssemblingMsgs {
		c.assemblingMsgs[i] = &msgToAssemble{
			msg: &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:           as.MsgID,
					Src:          as.Src,
					Dst:          as.Dst,
					RspTo:        as.RspTo,
					TrafficClass: as.TrafficClass,
					TrafficBytes: as.TrafficBytes,
				},
			},
			numFlitRequired: as.NumFlitRequired,
			numFlitArrived:  as.NumFlitArrived,
		}
	}

	c.assembledMsgs = make([]*sim.GenericMsg, len(s.AssembledMsgs))
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

func msgRefFromMsg(msg *sim.GenericMsg) msgRef {
	return msgRef{
		ID:           msg.ID,
		Src:          msg.Src,
		Dst:          msg.Dst,
		RspTo:        msg.RspTo,
		TrafficClass: msg.TrafficClass,
		TrafficBytes: msg.TrafficBytes,
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

func flitStateFromMsg(flit *sim.GenericMsg) flitState {
	payload := sim.MsgPayload[messaging.FlitPayload](flit)
	return flitState{
		ID:            flit.ID,
		Src:           flit.Src,
		Dst:           flit.Dst,
		SeqID:         payload.SeqID,
		NumFlitInMsg:  payload.NumFlitInMsg,
		OriginalMsgID: payload.Msg.ID,
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

func (m *middleware) flitTaskID(flitMsg *sim.GenericMsg) string {
	return fmt.Sprintf("%s_e2e", flitMsg.ID)
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

		msg := port.RetrieveOutgoing().(*sim.GenericMsg)
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

		received := receivedI.(*sim.GenericMsg)
		flitPayload := sim.MsgPayload[messaging.FlitPayload](received)
		msg := flitPayload.Msg

		var assembling *msgToAssemble
		for _, a := range m.assemblingMsgs {
			if a.msg.ID == msg.ID {
				assembling = a
				break
			}
		}

		if assembling == nil {
			assembling = &msgToAssemble{
				msg:             msg,
				numFlitRequired: flitPayload.NumFlitInMsg,
				numFlitArrived:  0,
			}
			m.assemblingMsgs = append(m.assemblingMsgs, assembling)
		}

		assembling.numFlitArrived++

		m.NetworkPort.RetrieveIncoming()

		m.logFlitE2ETask(received, true)

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
		dst := msg.Dst

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

func (m *middleware) logFlitE2ETask(flitMsg *sim.GenericMsg, isEnd bool) {
	if m.Comp.NumHooks() == 0 {
		return
	}

	flitPayload := sim.MsgPayload[messaging.FlitPayload](flitMsg)
	msg := flitPayload.Msg

	if isEnd {
		tracing.EndTask(m.flitTaskID(flitMsg), m.Comp)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(flitMsg), m.msgTaskID(msg.ID),
		m.Comp, "flit_e2e", "flit_e2e", m.Comp.Name()+".FlitBuf", flitMsg,
	)
}

func (m *middleware) logMsgE2ETask(msg *sim.GenericMsg, isEnd bool) {
	if m.Comp.NumHooks() == 0 {
		return
	}

	if msg.IsRsp() {
		m.logMsgRsp(isEnd, msg)
		return
	}

	m.logMsgReq(isEnd, msg)
}

func (m *middleware) logMsgReq(isEnd bool, msg *sim.GenericMsg) {
	if isEnd {
		tracing.EndTask(m.msgTaskID(msg.ID), m.Comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(msg.ID),
			msg.ID+"_req_out",
			m.Comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *middleware) logMsgRsp(isEnd bool, msg *sim.GenericMsg) {
	if isEnd {
		tracing.EndTask(m.msgTaskID(msg.ID), m.Comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(msg.ID),
			msg.RspTo+"_req_out",
			m.Comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *middleware) msgToFlits(msg *sim.GenericMsg) []*sim.GenericMsg {
	numFlit := 1

	if msg.TrafficBytes > 0 {
		trafficByte := msg.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * m.Comp.GetSpec().EncodingOverhead))
		numFlit = (trafficByte-1)/m.Comp.GetSpec().FlitByteSize + 1
	}

	flits := make([]*sim.GenericMsg, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = messaging.FlitBuilder{}.
			WithSrc(m.NetworkPort.AsRemote()).
			WithDst(m.DefaultSwitchDst).
			WithSeqID(i).
			WithNumFlitInMsg(numFlit).
			WithMsg(msg).
			Build()
	}

	return flits
}
