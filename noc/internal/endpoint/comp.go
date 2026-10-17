package endpoint

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the endpoint.
type Spec struct {
	Freq              timing.Freq `json:"freq"`
	NumInputChannels  int         `json:"num_input_channels"`
	NumOutputChannels int         `json:"num_output_channels"`
	FlitByteSize      int         `json:"flit_byte_size"`
	EncodingOverhead  float64     `json:"encoding_overhead"`
}

// Resources holds the endpoint's live device-side connection, created by Builder.
type Resources struct{ devices *deviceSide }

// assemblingMsgState is a serializable representation of a message being
// assembled from flits.
type assemblingMsgState struct {
	MsgID          uint64        `json:"msg_id"`
	MsgTaskID      uint64        `json:"msg_task_id"`
	Msg            messaging.Msg `json:"msg,omitzero"`
	Received       []bool        `json:"received"`
	NumFlitArrived int           `json:"num_flit_arrived"`
}

// state contains mutable runtime data for the endpoint.
type state struct {
	MsgOutBuf      []messaging.Msg      `json:"msg_out_buf"`
	FlitsToSend    []messaging.Msg      `json:"flits_to_send"`
	AssemblingMsgs []assemblingMsgState `json:"assembling_msgs"`
	AssembledMsgs  []messaging.Msg      `json:"assembled_msgs"`
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
	ports []messaging.Port
}

// DeviceConnection is the pass-through connection that device ports bind to.
func DeviceConnection(c *Comp) messaging.Connection { return c.Resources.devices }

// BindPort appends a device in arbitration order during setup.
func (d *deviceSide) BindPort(port messaging.Port) {
	d.RequireSetup()
	mustBeBufferedDevicePort(port)
	if port.Owner() == nil {
		panic("endpoint: device port has no owner")
	}
	if owner, ok := port.Owner().(interface{ RequireSetup() }); ok {
		owner.RequireSetup()
	}
	for _, existing := range d.ports {
		if existing == port {
			panic("endpoint: duplicate device port")
		}
	}
	if port.Connection() != nil {
		panic("endpoint: device port already connected")
	}
	port.BindConnection(d)
	d.ports = append(d.ports, port)
}

// NotifyAvailable wakes the endpoint when a device port has room again.
func (d *deviceSide) NotifyAvailable(_ messaging.Port) {
	d.TickLater()
}

// NotifySend wakes the endpoint when a device port has a message to send.
func (d *deviceSide) NotifySend() {
	d.TickLater()
}

// mustBeBufferedDevicePort enforces v5 NoC's supported push-buffered transport.
// The endpoint drains outgoing buffers and pushes reassembled messages through
// CanDeliver/Deliver, with notifications for new messages and available capacity.
// Planned wire ports use pull delivery and cannot support those push operations.
// Custom ports may provide these semantics, but v5 NoC supports twowaybuffered only.
func mustBeBufferedDevicePort(port messaging.Port) {
	if p, ok := port.(*twowaybuffered.Port); !ok || p == nil {
		panic(fmt.Sprintf("endpoint: device port must be *twowaybuffered.Port, got %T", port))
	}
}
