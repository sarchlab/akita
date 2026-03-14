package endpoint

import "github.com/sarchlab/akita/v5/sim"

// Spec contains immutable configuration for the endpoint.
type Spec struct {
	Freq              sim.Freq       `json:"freq"`
	NumInputChannels  int            `json:"num_input_channels"`
	NumOutputChannels int            `json:"num_output_channels"`
	FlitByteSize      int            `json:"flit_byte_size"`
	EncodingOverhead  float64        `json:"encoding_overhead"`
	DefaultSwitchDst  sim.RemotePort `json:"default_switch_dst"`
}
