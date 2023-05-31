package messaging

import "github.com/sarchlab/akita/v3/sim"

// MsgBuffer is a buffer that can hold requests
type MsgBuffer struct {
	Capacity int
	Buf      []sim.Msg
}
