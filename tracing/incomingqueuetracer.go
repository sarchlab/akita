package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// IncomingQueueTaskKind is the Kind of the task that records the time a message
// spends waiting *behind other messages* in a port's incoming buffer — from the
// moment the connection delivers it until it reaches the head of the buffer and
// becomes reachable by the owning component. Once it is at the head, any further
// delay is the component's own (it can see the message but cannot yet admit it),
// so it belongs to the component's processing task, not here.
const IncomingQueueTaskKind = "incoming_queue"

// incomingQueueHook records, for each message delivered to a port, a task that
// spans the message's wait behind earlier messages in the incoming buffer. The
// task is a child of the message's req_out task (the request's own ID, or, for a
// response, the ID it responds to). It is located at the receiving port and
// emitted on the port's owning component, so it flows to that component's
// tracers.
//
// The task opens on HookPosPortMsgRecvd (delivery) and closes when the message
// reaches the head of the buffer:
//   - immediately, if it is delivered into an empty buffer; or
//   - when the message ahead of it is retrieved (HookPosPortMsgRetrieveIncoming
//     exposes it as the new head).
//
// HookPosPortMsgRecvd fires while the port holds its lock, so the hook must not
// call back into the port there. It tracks the buffer depth itself (depth) to
// tell whether a freshly delivered message is already at the head. The incoming
// buffer is FIFO and the only push/pop paths are Deliver/RetrieveIncoming, both
// of which fire these hooks, so the mirrored depth stays exact.
type incomingQueueHook struct {
	mu      sync.Mutex
	taskIDs map[uint64]uint64 // message ID -> open queueing task ID
	depth   int               // messages currently in the port's incoming buffer
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

	switch ctx.Pos {
	case messaging.HookPosPortMsgRecvd:
		msg, ok := ctx.Item.(messaging.Msg)
		if !ok {
			return
		}
		h.onDeliver(domain, port, msg)
	case messaging.HookPosPortMsgRetrieveIncoming:
		h.onRetrieve(domain, port)
	}
}

// onDeliver opens the queueing task. A message delivered into an empty buffer is
// already at the head, so its behind-others interval is zero and the task is
// closed at once. Records nothing — and generates no ID — when the component is
// not being traced.
func (h *incomingQueueHook) onDeliver(
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
	atHead := h.depth == 0
	h.depth++
	h.mu.Unlock()

	StartTask(domain, TaskStart{
		ID:       id,
		ParentID: parentID,
		Kind:     IncomingQueueTaskKind,
		What:     msgTypeName(msg),
		Location: port.Name(),
	})

	if atHead {
		h.endByMsgID(domain, meta.ID)
	}
}

// onRetrieve removes the just-retrieved head and closes the queueing task of
// whatever message is now exposed at the head — it has stopped waiting behind
// others. The retrieve hook runs after the port releases its lock, so peeking
// the new head here is safe.
func (h *incomingQueueHook) onRetrieve(domain NamedHookable, port messaging.Port) {
	h.mu.Lock()
	if h.depth > 0 {
		h.depth--
	}
	h.mu.Unlock()

	newHead := port.PeekIncoming()
	if newHead == nil {
		return
	}

	h.endByMsgID(domain, newHead.Meta().ID)
}

// endByMsgID closes the queueing task for the message, if one is open. It is
// idempotent: a message whose task was already closed (e.g. it reached the head
// at delivery) is ignored.
func (h *incomingQueueHook) endByMsgID(domain NamedHookable, msgID uint64) {
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
