package endpoint

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the endpoint.
type Spec struct {
	Freq              timing.Freq          `json:"freq"`
	NumInputChannels  int                  `json:"num_input_channels"`
	NumOutputChannels int                  `json:"num_output_channels"`
	FlitByteSize      int                  `json:"flit_byte_size"`
	EncodingOverhead  float64              `json:"encoding_overhead"`
	DefaultSwitchDst  messaging.RemotePort `json:"default_switch_dst"`
}

// Resources holds the external wiring referenced by the endpoint, namely the
// device ports that communicate directly through it. These are remote ports
// owned by other components, so they belong in Resources rather than Spec.
type Resources struct {
	DevicePorts []messaging.Port `json:"-"`
}

// msgHolder carries one polymorphic messaging.Msg inside the endpoint's
// otherwise plain-JSON State. A bare interface field cannot be checkpointed —
// the JSON decoder cannot reconstruct the concrete type — so the holder encodes
// the message through the message codec (the same machinery that checkpoints
// in-flight messages in port buffers). A nil message round-trips as nil.
type msgHolder struct {
	Msg messaging.Msg
}

// MarshalJSON encodes the held message through the message codec.
func (h msgHolder) MarshalJSON() ([]byte, error) {
	return messaging.EncodeMsg(h.Msg)
}

// UnmarshalJSON decodes the held message through the message codec.
func (h *msgHolder) UnmarshalJSON(data []byte) error {
	msg, err := messaging.DecodeMsg(data)
	if err != nil {
		return err
	}

	h.Msg = msg

	return nil
}

// assemblingMsgState is a serializable record of a message being reassembled
// from flits. Arrival progress is tracked by message ID; the carried concrete
// message is captured once the flit bearing it (the final flit) arrives.
type assemblingMsgState struct {
	MsgID           uint64    `json:"msg_id"`
	NumFlitRequired int       `json:"num_flit_required"`
	NumFlitArrived  int       `json:"num_flit_arrived"`
	Payload         msgHolder `json:"payload"`
}

// State contains mutable runtime data for the endpoint. The message-bearing
// buffers carry concrete messages (wrapped in msgHolder so they checkpoint),
// so a payload-bearing protocol survives the network crossing.
type State struct {
	MsgOutBuf      []msgHolder          `json:"msg_out_buf"`
	FlitsToSend    []packetization.Flit `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []msgHolder          `json:"assembled_msgs"`
}

// Comp is an akita component(Endpoint) that delegates sending and receiving
// actions of a few ports.
type Comp struct {
	*modeling.Component[Spec, State, modeling.None]
}

// outgoingMW returns the outgoing middleware from the component's middleware list.
func (c *Comp) outgoingMW() *outgoingMW {
	return c.Middlewares()[0].(*outgoingMW)
}

// incomingMW returns the incoming middleware from the component's middleware list.
func (c *Comp) incomingMW() *incomingMW {
	return c.Middlewares()[1].(*incomingMW)
}

// NetworkPort returns the network port of the endpoint. It panics if the port
// has not been assigned yet (see SetNetworkPort).
func (c *Comp) NetworkPort() messaging.Port {
	return c.GetPortByName("NetworkPort")
}

// SetNetworkPort assigns the endpoint's network-port instance. The endpoint
// declares the "NetworkPort" in Build; callers (e.g. the network connector)
// create the real port from the built endpoint and assign it here.
func (c *Comp) SetNetworkPort(p messaging.Port) {
	c.AssignPort("NetworkPort", p)
}

// SetDefaultSwitchDst sets the default switch destination. Prefer the
// WithDefaultSwitchDst builder option for build-time wiring. This setter remains
// for callers that only learn the destination after the endpoint is built (e.g.
// the network connector creates the switch port from the built endpoint, and
// two endpoints connected directly each need the other's post-build port).
func (c *Comp) SetDefaultSwitchDst(dst messaging.RemotePort) {
	c.outgoingMW().defaultSwitchDst = dst
}

// PlugIn connects a port to the endpoint.
func (c *Comp) PlugIn(port messaging.Port) {
	port.SetConnection(c)
	c.outgoingMW().devicePorts = append(c.outgoingMW().devicePorts, port)
	c.incomingMW().devicePorts = append(c.incomingMW().devicePorts, port)
}

// NotifyAvailable triggers the endpoint to continue to tick.
func (c *Comp) NotifyAvailable(_ messaging.Port) {
	c.TickLater()
}

// NotifySend is called by a port to notify the connection there are
// messages waiting to be sent, can start tick
func (c *Comp) NotifySend() {
	c.TickLater()
}

// Unplug removes the association of a port and an endpoint.
func (c *Comp) Unplug(_ messaging.Port) {
	panic("not implemented")
}
