package switches

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the switch.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}

// routedFlit is a flit that has been received and assigned a route destination.
type routedFlit struct {
	packetization.Flit
	TaskID       uint64               `json:"task_id"`
	RouteTo      messaging.RemotePort `json:"route_to"`
	OutputBufIdx int                  `json:"output_buf_idx"`
}

// portComplexState is the serializable state of one port complex.
type portComplexState struct {
	LocalPortName    string                              `json:"local_port_name"`
	RemotePort       messaging.RemotePort                `json:"remote_port"`
	NumInputChannel  int                                 `json:"num_input_channel"`
	NumOutputChannel int                                 `json:"num_output_channel"`
	Latency          int                                 `json:"latency"`
	PipelineWidth    int                                 `json:"pipeline_width"`
	Pipeline         queueing.Pipeline[routedFlit]       `json:"pipeline"`
	RouteBuffer      queueing.Buffer[routedFlit]         `json:"route_buffer"`
	ForwardBuffer    queueing.Buffer[routedFlit]         `json:"forward_buffer"`
	SendOutBuffer    queueing.Buffer[packetization.Flit] `json:"send_out_buffer"`
}

// State contains mutable runtime data for the switch.
type State struct {
	PortComplexes []portComplexState `json:"port_complexes"`
}

// Comp is the switch component.
type Comp = modeling.Component[Spec, State, modeling.None]
