package switches

import (
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// routedFlit is a flit that has been received and assigned a route destination.
type routedFlit struct {
	messaging.Flit
	TaskID  string         `json:"task_id"`
	RouteTo sim.RemotePort `json:"route_to"`
}

// portComplexState is the serializable state of one port complex.
type portComplexState struct {
	LocalPortName    string                           `json:"local_port_name"`
	RemotePort       sim.RemotePort                   `json:"remote_port"`
	NumInputChannel  int                              `json:"num_input_channel"`
	NumOutputChannel int                              `json:"num_output_channel"`
	Latency          int                              `json:"latency"`
	PipelineWidth    int                              `json:"pipeline_width"`
	Pipeline         queueing.Pipeline[routedFlit]   `json:"pipeline"`
	RouteBuffer      queueing.Buffer[routedFlit]     `json:"route_buffer"`
	ForwardBuffer    queueing.Buffer[routedFlit]     `json:"forward_buffer"`
	SendOutBuffer    queueing.Buffer[messaging.Flit] `json:"send_out_buffer"`
}

// State contains mutable runtime data for the switch.
type State struct {
	PortComplexes []portComplexState `json:"port_complexes"`
}
