package control

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
)

// IsResetPending reports whether the message currently waiting at the head of
// the given control port is a Reset request.
//
// Reset is the highest-priority verb: a queued Reset preempts the completion
// of an in-progress async verb (Drain/Flush), so a stale async ack is never
// sent ahead of it. Any other verb instead lets the pending async verb finish
// first. Control middlewares use this to skip completing a pending drain only
// when a Reset is the next thing to service.
func IsResetPending(port messaging.Port) bool {
	msg := port.PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(mem.ControlReq)

	return ok && req.Command == mem.CmdReset
}
