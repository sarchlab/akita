package packetization

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// Flit is a concrete message representing the smallest transferring unit on a
// network.
type Flit struct {
	messaging.MsgMeta
	SeqID        int               `json:"seq_id"`
	NumFlitInMsg int               `json:"num_flit_in_msg"`
	Msg          messaging.MsgMeta `json:"msg"`            // carried message metadata
	OutputBufIdx int               `json:"output_buf_idx"` // output buffer index within a switch
}
