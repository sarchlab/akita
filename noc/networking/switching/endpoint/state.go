package endpoint

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/packetization"
)

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
