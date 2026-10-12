package tracing

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
)

// IncomingBufferTaskKind records a message's residency in the incoming buffer.
const IncomingBufferTaskKind = "incoming_buffer"

type incomingBufferHook struct{}

func (h *incomingBufferHook) Func(ctx hooking.HookCtx) {
	switch ctx.Pos {
	case messaging.HookPosPortMsgRecvd, messaging.HookPosPortMsgRetrieveIncoming:
	default:
		return
	}
	port := ctx.Domain.(messaging.Port)
	msg := mustItem[messaging.Msg](ctx)
	domain, ok := port.Owner().(NamedHookable)
	if !ok || domain.NumHooks() == 0 {
		return
	}
	trace := ctx.Detail.(messaging.PortBufferTrace)
	if ctx.Pos == messaging.HookPosPortMsgRecvd && trace.TaskID != 0 {
		StartTask(domain, TaskStart{
			ID: trace.TaskID, MsgID: msg.ID, Kind: IncomingBufferTaskKind,
			What: msgTypeName(msg), Location: port.Name() + ".incoming", Detail: msg,
		})
	} else {
		EndTaskOnReset(domain, trace.TaskID)
	}
	if trace.HeadTaskID != 0 {
		AddMilestone(domain, Milestone{TaskID: trace.HeadTaskID, Kind: MilestoneKindQueue, What: port.Name()})
	}
}

// CollectIncomingBufferTrace attaches a stateless hook to a port. Buffer task
// IDs and head transitions come from the port, including after restoration.
// The tasks flow to tracers attached to the port's owner.
func CollectIncomingBufferTrace(port naming.Named) {
	p, ok := port.(messaging.Port)
	if !ok {
		return
	}
	p.AcceptHook(&incomingBufferHook{})
}
