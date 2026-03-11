package messaging

import (
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// Flit is a concrete message representing the smallest transferring unit on a
// network.
type Flit struct {
	sim.MsgMeta
	SeqID        int
	NumFlitInMsg int
	Msg          sim.Msg
	OutputBuf    queueing.Buffer // The buffer to route to within a switch
}
