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
	NumInputChannels  int     `json:"num_input_channels"`
	NumOutputChannels int     `json:"num_output_channels"`
	FlitByteSize      int     `json:"flit_byte_size"`
	EncodingOverhead  float64 `json:"encoding_overhead"`
}

// msgRef is a serializable representation of a *sim.Msg.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// flitState is a serializable representation of a flit *sim.Msg.
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
	msg             *sim.Msg
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

	msgOutBuf   []*sim.Msg
	flitsToSend []*sim.Msg

	assemblingMsgs []*msgToAssemble
	assembledMsgs  []*sim.Msg
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

// SyncState copies mutable runtime data into the State struct.
func (c *Comp) SyncState() {
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

	c.SetState(s)
}

func msgRefFromMsg(msg *sim.Msg) msgRef {
	return msgRef{
		ID:           msg.ID,
		Src:          msg.Src,
		Dst:          msg.Dst,
		RspTo:        msg.RspTo,
		TrafficClass: msg.TrafficClass,
		TrafficBytes: msg.TrafficBytes,
	}
}

func flitStateFromMsg(flit *sim.Msg) flitState {
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

func (m *middleware) flitTaskID(flitMsg *sim.Msg) string {
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
		received := m.NetworkPort.PeekIncoming()
		if received == nil {
			return madeProgress
		}

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

func (m *middleware) logFlitE2ETask(flitMsg *sim.Msg, isEnd bool) {
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

func (m *middleware) logMsgE2ETask(msg *sim.Msg, isEnd bool) {
	if m.Comp.NumHooks() == 0 {
		return
	}

	if msg.IsRsp() {
		m.logMsgRsp(isEnd, msg)
		return
	}

	m.logMsgReq(isEnd, msg)
}

func (m *middleware) logMsgReq(isEnd bool, msg *sim.Msg) {
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

func (m *middleware) logMsgRsp(isEnd bool, msg *sim.Msg) {
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

func (m *middleware) msgToFlits(msg *sim.Msg) []*sim.Msg {
	numFlit := 1

	if msg.TrafficBytes > 0 {
		trafficByte := msg.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * m.Comp.GetSpec().EncodingOverhead))
		numFlit = (trafficByte-1)/m.Comp.GetSpec().FlitByteSize + 1
	}

	flits := make([]*sim.Msg, numFlit)
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
