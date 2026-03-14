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
	Freq              sim.Freq       `json:"freq"`
	NumInputChannels  int            `json:"num_input_channels"`
	NumOutputChannels int            `json:"num_output_channels"`
	FlitByteSize      int            `json:"flit_byte_size"`
	EncodingOverhead  float64        `json:"encoding_overhead"`
	DefaultSwitchDst  sim.RemotePort `json:"default_switch_dst"`
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
	FlitsToSend    []messaging.Flit     `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []sim.MsgMeta        `json:"assembled_msgs"`
}

// Comp is an akita component(Endpoint) that delegates sending and receiving
// actions of a few ports.
type Comp struct {
	*modeling.Component[Spec, State]
}

// outgoingMW returns the outgoing middleware from the component's middleware list.
func (c *Comp) outgoingMW() *outgoingMW {
	return c.Middlewares()[0].(*outgoingMW)
}

// incomingMW returns the incoming middleware from the component's middleware list.
func (c *Comp) incomingMW() *incomingMW {
	return c.Middlewares()[1].(*incomingMW)
}

// NetworkPort returns the network port of the endpoint.
func (c *Comp) NetworkPort() sim.Port {
	return c.outgoingMW().networkPort
}

// SetNetworkPort sets the network port of the endpoint.
func (c *Comp) SetNetworkPort(p sim.Port) {
	c.outgoingMW().networkPort = p
	c.incomingMW().networkPort = p
}

// DefaultSwitchDst returns the default switch destination.
func (c *Comp) DefaultSwitchDst() sim.RemotePort {
	return c.outgoingMW().defaultSwitchDst
}

// SetDefaultSwitchDst sets the default switch destination.
func (c *Comp) SetDefaultSwitchDst(dst sim.RemotePort) {
	c.outgoingMW().defaultSwitchDst = dst
}

// PlugIn connects a port to the endpoint.
func (c *Comp) PlugIn(port sim.Port) {
	port.SetConnection(c)
	c.outgoingMW().devicePorts = append(c.outgoingMW().devicePorts, port)
	c.incomingMW().devicePorts = append(c.incomingMW().devicePorts, port)
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

// msgMetaToFlits converts a MsgMeta into a slice of messaging.Flit entries.
func msgMetaToFlits(
	meta sim.MsgMeta,
	spec Spec,
	networkPortRemote sim.RemotePort,
	defaultSwitchDst sim.RemotePort,
) []messaging.Flit {
	numFlit := 1
	if meta.TrafficBytes > 0 {
		trafficByte := meta.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * spec.EncodingOverhead))
		numFlit = (trafficByte-1)/spec.FlitByteSize + 1
	}

	flits := make([]messaging.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = messaging.Flit{
			MsgMeta: sim.MsgMeta{
				ID:  fmt.Sprintf("flit-%d-msg-%s-%s", i, meta.ID, sim.GetIDGenerator().Generate()),
				Src: networkPortRemote,
				Dst: defaultSwitchDst,
			},
			SeqID:        i,
			NumFlitInMsg: numFlit,
			Msg: sim.MsgMeta{
				ID:           meta.ID,
				Src:          meta.Src,
				Dst:          meta.Dst,
				RspTo:        meta.RspTo,
				TrafficClass: meta.TrafficClass,
				TrafficBytes: meta.TrafficBytes,
			},
		}
	}

	return flits
}

// outgoingMW handles the device→network path:
// sendFlitOut, prepareMsg, prepareFlits.
type outgoingMW struct {
	comp             *modeling.Component[Spec, State]
	devicePorts      []sim.Port
	networkPort      sim.Port
	defaultSwitchDst sim.RemotePort
}

// Tick runs the outgoing stages.
func (m *outgoingMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendFlitOut() || madeProgress
	madeProgress = m.prepareMsg() || madeProgress
	madeProgress = m.prepareFlits() || madeProgress

	return madeProgress
}

func (m *outgoingMW) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (m *outgoingMW) flitTaskID(flitID string) string {
	return fmt.Sprintf("%s_e2e", flitID)
}

func (m *outgoingMW) sendFlitOut() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()

	numSent := 0

	for i := 0; i < spec.NumOutputChannels; i++ {
		if numSent >= len(cur.FlitsToSend) {
			break
		}

		flit := &cur.FlitsToSend[numSent]

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

// maxMsgOutBufSize limits the number of messages buffered before flit
// conversion. This prevents the serialisable state from growing
// unboundedly and causing slow deep copies.
const maxMsgOutBufSize = 16

func (m *outgoingMW) prepareMsg() bool {
	madeProgress := false
	next := m.comp.GetNextState()

	for i := 0; i < len(m.devicePorts); i++ {
		// Backpressure: stop accepting new messages when the outgoing
		// message buffer is already large enough.
		if len(next.MsgOutBuf) >= maxMsgOutBufSize {
			break
		}

		port := m.devicePorts[i]
		if port.PeekOutgoing() == nil {
			continue
		}

		msg := port.RetrieveOutgoing()
		next.MsgOutBuf = append(next.MsgOutBuf, *msg.Meta())

		madeProgress = true
	}

	return madeProgress
}

// maxFlitsToBuffer limits the number of flits held in FlitsToSend at once.
// This prevents the serialisable state from growing unboundedly, which would
// make the deep copy in modeling.Component.Tick() extremely slow.
const maxFlitsToBuffer = 64

func (m *outgoingMW) prepareFlits() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	networkPortRemote := m.networkPort.AsRemote()

	for {
		if len(next.MsgOutBuf) == 0 {
			return madeProgress
		}

		// Apply backpressure: don't convert more messages to flits while
		// the flit send buffer is already large.
		if len(next.FlitsToSend) >= maxFlitsToBuffer {
			return madeProgress
		}

		meta := next.MsgOutBuf[0]
		next.MsgOutBuf = next.MsgOutBuf[1:]
		flits := msgMetaToFlits(meta, spec, networkPortRemote, m.defaultSwitchDst)
		next.FlitsToSend = append(next.FlitsToSend, flits...)

		for _, fs := range flits {
			m.logFlitE2ETask(fs, false, &meta)
		}

		madeProgress = true
	}
}

func (m *outgoingMW) logFlitE2ETask(
	fs messaging.Flit, isEnd bool, meta *sim.MsgMeta,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.flitTaskID(fs.ID), m.comp)
		return
	}

	flit := &messaging.Flit{
		MsgMeta:      fs.MsgMeta,
		SeqID:        fs.SeqID,
		NumFlitInMsg: fs.NumFlitInMsg,
		Msg:          *meta,
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(fs.ID), m.msgTaskID(meta.ID),
		m.comp, "flit_e2e", "flit_e2e", m.comp.Name()+".FlitBuf", flit,
	)
}

// incomingMW handles the network→device path:
// tryDeliver, assemble, recv.
type incomingMW struct {
	comp        *modeling.Component[Spec, State]
	devicePorts []sim.Port
	networkPort sim.Port
}

// Tick runs the incoming stages.
func (m *incomingMW) Tick() bool {
	madeProgress := false

	madeProgress = m.tryDeliver() || madeProgress
	madeProgress = m.assemble() || madeProgress
	madeProgress = m.recv() || madeProgress

	return madeProgress
}

func (m *incomingMW) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (m *incomingMW) flitTaskID(flitID string) string {
	return fmt.Sprintf("%s_e2e", flitID)
}

func (m *incomingMW) recv() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	for i := 0; i < spec.NumInputChannels; i++ {
		receivedI := m.networkPort.PeekIncoming()
		if receivedI == nil {
			return madeProgress
		}

		flit := receivedI.(*messaging.Flit)
		msg := &flit.Msg

		var assemblingIdx int = -1
		for j, a := range next.AssemblingMsgs {
			if a.MsgID == msg.ID {
				assemblingIdx = j
				break
			}
		}

		if assemblingIdx < 0 {
			next.AssemblingMsgs = append(next.AssemblingMsgs, assemblingMsgState{
				MsgID:           msg.ID,
				Src:             msg.Src,
				Dst:             msg.Dst,
				RspTo:           msg.RspTo,
				TrafficClass:    msg.TrafficClass,
				TrafficBytes:    msg.TrafficBytes,
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

func (m *incomingMW) assemble() bool {
	madeProgress := false
	cur := m.comp.GetState()

	if len(cur.AssemblingMsgs) == 0 {
		return false
	}

	next := m.comp.GetNextState()

	// Compact in-place: move incomplete entries to the front.
	writeIdx := 0

	for i := range next.AssemblingMsgs {
		a := &next.AssemblingMsgs[i]
		if a.NumFlitArrived < a.NumFlitRequired {
			if writeIdx != i {
				next.AssemblingMsgs[writeIdx] = *a
			}
			writeIdx++
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

	next.AssemblingMsgs = next.AssemblingMsgs[:writeIdx]

	return madeProgress
}

func (m *incomingMW) tryDeliver() bool {
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

func (m *incomingMW) logFlitE2ETaskFromFlit(
	flit *messaging.Flit, isEnd bool,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.flitTaskID(flit.ID), m.comp)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(flit.ID), m.msgTaskID(flit.Msg.ID),
		m.comp, "flit_e2e", "flit_e2e", m.comp.Name()+".FlitBuf", flit,
	)
}

func (m *incomingMW) logMsgE2ETask(msg sim.Msg, isEnd bool) {
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

func (m *incomingMW) logMsgReq(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m.comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.ID+"_req_out",
			m.comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (m *incomingMW) logMsgRsp(isEnd bool, msg sim.Msg) {
	meta := msg.Meta()
	if isEnd {
		tracing.EndTask(m.msgTaskID(meta.ID), m.comp)
	} else {
		tracing.StartTask(
			m.msgTaskID(meta.ID),
			meta.RspTo+"_req_out",
			m.comp, "msg_e2e", "msg_e2e", msg,
		)
	}
}
