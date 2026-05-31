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

// assemblingMsgState is a serializable representation of a message being
// assembled from flits.
type assemblingMsgState struct {
	MsgID           uint64               `json:"msg_id"`
	Src             messaging.RemotePort `json:"src"`
	Dst             messaging.RemotePort `json:"dst"`
	RspTo           uint64               `json:"rsp_to"`
	TrafficClass    string               `json:"traffic_class"`
	TrafficBytes    int                  `json:"traffic_bytes"`
	NumFlitRequired int                  `json:"num_flit_required"`
	NumFlitArrived  int                  `json:"num_flit_arrived"`
}

// State contains mutable runtime data for the endpoint.
type State struct {
	MsgOutBuf      []messaging.MsgMeta  `json:"msg_out_buf"`
	FlitsToSend    []packetization.Flit `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []messaging.MsgMeta  `json:"assembled_msgs"`
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

// NetworkPort returns the network port of the endpoint.
func (c *Comp) NetworkPort() messaging.Port {
	return c.outgoingMW().networkPort
}

// SetNetworkPort sets the network port of the endpoint.
func (c *Comp) SetNetworkPort(p messaging.Port) {
	c.outgoingMW().networkPort = p
	c.incomingMW().networkPort = p
}

// DefaultSwitchDst returns the default switch destination.
func (c *Comp) DefaultSwitchDst() messaging.RemotePort {
	return c.outgoingMW().defaultSwitchDst
}

// SetDefaultSwitchDst sets the default switch destination.
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
