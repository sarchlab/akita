package endpoint

import (
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
)

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
