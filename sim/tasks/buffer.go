package tasks

import (
	"github.com/sarchlab/akita/v5/sim/internal/taskcore"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// AddBufferMilestone resolves an incoming-buffer task by message identity. Call
// before RetrieveIncoming. Unknown messages are ignored, never attributed to a
// later tracked message.
func AddBufferMilestone(o Owner, p messaging.Port, msg messaging.Msg, m Milestone) {
	taskcore.AddBufferMilestone(o, p, msg, m)
}
