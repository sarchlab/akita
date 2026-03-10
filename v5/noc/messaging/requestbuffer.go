package messaging

import "github.com/sarchlab/akita/v5/sim"

// MsgBuffer is a buffer that can hold requests
type MsgBuffer struct {
	Capacity int
	Buf      []*sim.Msg
}
