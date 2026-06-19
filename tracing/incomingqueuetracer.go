package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// IncomingQueueTaskKind is the Kind of the task that records the time a message
// spends waiting in a port's incoming buffer, between the moment the connection
// delivers it to the port and the moment the owning component takes it out.
const IncomingQueueTaskKind = "incoming_queue"

// incomingQueueHook records, for each message delivered to a port, a task that
// spans the message's stay in the port's incoming buffer. The task is a child
// of the message's req_out task — the request's own ID, or, for a response, the
// ID it responds to — so it nests inside the round trip as a sibling of the
// component's processing (req_in) task.
//
// Start and end are driven by the port's own hook positions:
// HookPosPortMsgRecvd fires when the connection delivers the message, and
// HookPosPortMsgRetrieveIncoming fires when the component retrieves it. The two
// are paired by message ID. Tasks are emitted on the port's owning component,
// so they flow to whatever tracers are collecting from that component.
type incomingQueueHook struct {
	mu      sync.Mutex
	taskIDs map[uint64]uint64 // message ID -> queueing task ID
}

// Func implements hooking.Hook.
func (h *incomingQueueHook) Func(ctx hooking.HookCtx) {
	port, ok := ctx.Domain.(messaging.Port)
	if !ok {
		return
	}

	domain, ok := port.Component().(NamedHookable)
	if !ok {
		return
	}

	msg, ok := ctx.Item.(messaging.Msg)
	if !ok {
		return
	}

	switch ctx.Pos {
	case messaging.HookPosPortMsgRecvd:
		h.startQueueTask(domain, port, msg)
	case messaging.HookPosPortMsgRetrieveIncoming:
		h.endQueueTask(domain, msg)
	}
}

// startQueueTask opens the queueing task when a message is delivered. It records
// nothing — and generates no ID — when the component is not being traced, so
// untraced runs pay only the cost of the NumHooks check.
func (h *incomingQueueHook) startQueueTask(
	domain NamedHookable,
	port messaging.Port,
	msg messaging.Msg,
) {
	if domain.NumHooks() == 0 {
		return
	}

	meta := msg.Meta()

	parentID := meta.ID
	if meta.IsRsp() {
		parentID = meta.RspTo
	}

	id := timing.GetIDGenerator().Generate()

	h.mu.Lock()
	h.taskIDs[meta.ID] = id
	h.mu.Unlock()

	// Located at the port, not the owning component: the wait happens in this
	// specific port's incoming buffer, so a per-port location is more precise
	// than the component name StartTask would otherwise default to.
	StartTask(domain, TaskStart{
		ID:       id,
		ParentID: parentID,
		Kind:     IncomingQueueTaskKind,
		What:     msgTypeName(msg),
		Location: port.Name(),
	})
}

// endQueueTask closes the queueing task when the component retrieves the
// message. Unmatched retrievals (e.g. a message delivered before tracing began)
// are ignored.
func (h *incomingQueueHook) endQueueTask(
	domain NamedHookable,
	msg messaging.Msg,
) {
	msgID := msg.Meta().ID

	h.mu.Lock()
	id, found := h.taskIDs[msgID]
	delete(h.taskIDs, msgID)
	h.mu.Unlock()

	if !found {
		return
	}

	EndTask(domain, TaskEnd{ID: id})
}

// CollectIncomingQueueTrace attaches incoming-buffer queueing tracing to a port.
// It accepts any named entity and is a no-op for anything that is not a
// [messaging.Port], so callers that handle entities uniformly can pass them
// through without a type switch. The emitted tasks flow to the tracers
// collecting from the port's owning component, so this is only effective once
// that component is itself being traced (see [CollectTrace]).
func CollectIncomingQueueTrace(port naming.Named) {
	p, ok := port.(messaging.Port)
	if !ok {
		return
	}

	p.AcceptHook(&incomingQueueHook{
		taskIDs: make(map[uint64]uint64),
	})
}
