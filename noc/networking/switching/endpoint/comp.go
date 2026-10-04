package endpoint

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the endpoint.
type Spec struct {
	Freq              timing.Freq `json:"freq"`
	NumInputChannels  int         `json:"num_input_channels"`
	NumOutputChannels int         `json:"num_output_channels"`
	FlitByteSize      int         `json:"flit_byte_size"`
	EncodingOverhead  float64     `json:"encoding_overhead"`

	// DefaultSwitchDst is the port at the other end of the network link,
	// usually a switch port, that every flit is sent to.
	DefaultSwitchDst messaging.RemotePort `json:"default_switch_dst"`
}

// Resources holds the external wiring referenced by the endpoint, namely the
// device ports that communicate directly through it. These are ports owned by
// other components; Build plugs them into the endpoint, which acts as their
// connection.
type Resources struct {
	DevicePorts []messaging.Port `json:"-"`
}

// assemblingMsgState is a serializable representation of a message being
// assembled from flits.
type assemblingMsgState struct {
	MsgID           uint64               `json:"msg_id"`
	MsgTaskID       uint64               `json:"msg_task_id"`
	Src             messaging.RemotePort `json:"src"`
	Dst             messaging.RemotePort `json:"dst"`
	RspTo           uint64               `json:"rsp_to"`
	TrafficClass    string               `json:"traffic_class"`
	TrafficBytes    int                  `json:"traffic_bytes"`
	NumFlitRequired int                  `json:"num_flit_required"`
	NumFlitArrived  int                  `json:"num_flit_arrived"`
}

// state contains mutable runtime data for the endpoint.
type state struct {
	MsgOutBuf      []messaging.MsgMeta  `json:"msg_out_buf"`
	FlitsToSend    []packetization.Flit `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []messaging.MsgMeta  `json:"assembled_msgs"`
}

// Ports holds the endpoint's ports.
type Ports struct {
	// NetworkPort exchanges flits with the switch (or the endpoint) on the
	// other end of the link.
	NetworkPort messaging.Port `akita:"role=github.com/sarchlab/akita/v5/noc/packetization.link"`
}

// middlewares holds the endpoint's behavior, run in field order every cycle.
type middlewares struct {
	// Outgoing takes messages from the device ports, splits them into flits,
	// and sends the flits out of the network port.
	Outgoing *outgoingMW

	// Incoming reassembles flits from the network port into messages and
	// delivers them to the device ports.
	Incoming *incomingMW
}

// Comp is an endpoint: it carries the messages of a few device ports over the
// network as flits.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]

// deviceSide is the connection the device ports plug into. It is part of the
// endpoint: its name and hooks are the endpoint's, and activity on a device
// port wakes the endpoint.
type deviceSide struct {
	*Comp
}

// PlugIn connects a device port to the endpoint.
func (d deviceSide) PlugIn(port messaging.Port) {
	port.SetConnection(d)
}

// NotifyAvailable wakes the endpoint when a device port has room again.
func (d deviceSide) NotifyAvailable(_ messaging.Port) {
	d.TickLater()
}

// NotifySend wakes the endpoint when a device port has a message to send.
func (d deviceSide) NotifySend() {
	d.TickLater()
}
