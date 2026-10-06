package switches

import (
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the switch.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}

// Resources holds the external wiring referenced by the switch: the routing
// table used to resolve flit destinations, and the link behind each port.
// Both are supplied by the system builder and rebuilt, not saved, on restore.
type Resources struct {
	RoutingTable routing.Table `json:"-"`

	// Links describes the link behind each port, index-aligned with
	// Ports.Port.
	Links []Link `json:"-"`
}

// Link describes the link behind one switch port.
type Link struct {
	// Remote is the port at the other end of the link: an endpoint's network
	// port or another switch's port.
	Remote messaging.RemotePort

	// Latency is the number of cycles a flit spends in the switch after
	// entering from this port.
	Latency int

	// NumInputChannel is the number of flits the port can inject into the
	// switch per cycle.
	NumInputChannel int

	// NumOutputChannel is the number of flits the switch can eject to the
	// port per cycle.
	NumOutputChannel int
}

// Ports holds the switch's ports.
type Ports struct {
	// Port holds one port per link, index-aligned with Resources.Links.
	Port []messaging.Port `akita:"role=github.com/sarchlab/akita/v5/noc/packetization.link"`
}

// middlewares holds the switch's behavior, run in field order every cycle.
type middlewares struct {
	// RouteForwardSend sends flits out, forwards them to their output
	// buffers, and routes them.
	RouteForwardSend *routeForwardSendMW

	// ReceivePipeline moves flits through the per-port pipelines and takes
	// new flits in.
	ReceivePipeline *receivePipelineMW
}

// routedFlit is a flit that has been received and assigned a route destination.
type routedFlit struct {
	Flit         messaging.Msg
	TaskID       uint64               `json:"task_id"`
	RouteTo      messaging.RemotePort `json:"route_to"`
	OutputBufIdx int                  `json:"output_buf_idx"`
}

// portComplexState is the serializable state of one port complex.
type portComplexState struct {
	LocalPortName    string                        `json:"local_port_name"`
	RemotePort       messaging.RemotePort          `json:"remote_port"`
	NumInputChannel  int                           `json:"num_input_channel"`
	NumOutputChannel int                           `json:"num_output_channel"`
	Latency          int                           `json:"latency"`
	PipelineWidth    int                           `json:"pipeline_width"`
	Pipeline         queueing.Pipeline[routedFlit] `json:"pipeline"`
	RouteBuffer      queueing.Buffer[routedFlit]   `json:"route_buffer"`
	ForwardBuffer    queueing.Buffer[routedFlit]   `json:"forward_buffer"`
	// SendOutBuffer holds routedFlits (not bare Flits) so the in-switch "flit"
	// task can stay open until the flit is actually sent out the port, charging
	// the time the flit waits here for the output link.
	SendOutBuffer queueing.Buffer[routedFlit] `json:"send_out_buffer"`
}

// state contains mutable runtime data for the switch.
type state struct {
	PortComplexes []portComplexState `json:"port_complexes"`

	// NextArbPort is the round-robin arbitration cursor for forwarding. It is
	// runtime state (not topology), so it lives in State to be checkpointed;
	// otherwise a resumed switch would restart arbitration from port 0 and
	// diverge from an uninterrupted run.
	NextArbPort int `json:"next_arb_port"`
}

// Comp is the switch component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
