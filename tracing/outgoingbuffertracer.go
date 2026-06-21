package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
)

// OutgoingBufferTaskKind is the Kind of the task that records how long a message
// sits in a port's outgoing buffer — from the moment the owning component sends
// it until the connection retrieves (drains) it onto the link. The task carries
// a "reached head" milestone (emitted by the hook the instant the message
// becomes the head of the buffer) that ends the waiting-behind-others interval;
// the remaining time at the head, until the connection drains it, is the wait
// for the link to become free. The component's own work that produced the
// message is the separate req_out task, whose ID this task is parented to.
const OutgoingBufferTaskKind = "outgoing_buffer"

// outgoingBufferHook records, for each message sent from a port, a buffer task
// spanning the message's residency in the outgoing buffer. The task is a child
// of the message's req_out task (the request's own ID, or, for a response, the
// ID it responds to). It is located at the sending port and emitted on the
// port's owning component, so it flows to that component's tracers.
//
// The task opens on HookPosPortMsgSend (enqueue) and closes on
// HookPosPortMsgRetrieveOutgoing (drain by the connection). The hook emits a
// "reached head" milestone the instant the message becomes the head of the
// buffer:
//   - immediately, if it is sent into an empty buffer; or
//   - when the message ahead of it is drained, exposing it as the new head.
//
// Reaching the head is detected here, from the buffer's own push/pop events, so
// it is exact.
//
// HookPosPortMsgSend fires while the port holds its lock, so the hook must not
// call back into the port there. It tracks the buffer depth itself (depth) to
// tell whether a freshly sent message is already at the head. The outgoing
// buffer is FIFO and the only push/pop paths are Send/RetrieveOutgoing, both of
// which fire these hooks, so the mirrored depth stays exact.
type outgoingBufferHook struct {
	mu    sync.Mutex
	depth int // messages currently in the port's outgoing buffer
}

// Func implements hooking.Hook.
func (h *outgoingBufferHook) Func(ctx hooking.HookCtx) {
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
	case messaging.HookPosPortMsgSend:
		h.onSend(domain, port, msg)
	case messaging.HookPosPortMsgRetrieveOutgoing:
		h.onRetrieve(domain, port, msg)
	}
}

// onSend opens the buffer task. A message sent into an empty buffer is already
// at the head, so its reached-head milestone fires at once (its
// waiting-behind-others interval is zero). Records nothing — and generates no
// ID — when the component is not being traced.
func (h *outgoingBufferHook) onSend(
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

	id := MsgIDAtOutgoingBuffer(msg, domain)

	h.mu.Lock()
	atHead := h.depth == 0
	h.depth++
	h.mu.Unlock()

	StartTask(domain, TaskStart{
		ID:       id,
		ParentID: parentID,
		Kind:     OutgoingBufferTaskKind,
		What:     msgTypeName(msg),
		Location: port.Name(),
	})

	if atHead {
		h.markReachedHead(domain, port, id)
	}
}

// onRetrieve closes the drained message's buffer task and, if the buffer is not
// now empty, marks the message exposed as the new head as having reached it.
// The retrieve hook runs after the port releases its lock, so peeking the new
// head here is safe.
func (h *outgoingBufferHook) onRetrieve(
	domain NamedHookable,
	port messaging.Port,
	retrieved messaging.Msg,
) {
	if domain.NumHooks() == 0 {
		return
	}

	h.mu.Lock()
	if h.depth > 0 {
		h.depth--
	}
	h.mu.Unlock()

	EndTask(domain, TaskEnd{ID: MsgIDAtOutgoingBuffer(retrieved, domain)})
	ForgetMsgIDAtOutgoingBuffer(retrieved.Meta().ID, domain)

	newHead := port.PeekOutgoing()
	if newHead == nil {
		return
	}

	h.markReachedHead(domain, port, MsgIDAtOutgoingBuffer(newHead, domain))
}

// markReachedHead records that a message has reached the head of the buffer and
// stopped waiting behind earlier messages. Any further delay before the
// connection drains it is the wait for the link to become free.
func (h *outgoingBufferHook) markReachedHead(
	domain NamedHookable, port messaging.Port, taskID uint64,
) {
	AddMilestone(domain, Milestone{
		TaskID: taskID,
		Kind:   MilestoneKindQueue,
		What:   port.Name(),
	})
}

// CollectOutgoingBufferTrace attaches outgoing-buffer tracing to a port. It
// accepts any named entity and is a no-op for anything that is not a
// [messaging.Port], so callers that handle entities uniformly can pass them
// through without a type switch. The emitted tasks flow to the tracers
// collecting from the port's owning component, so this is only effective once
// that component is itself being traced (see [CollectTrace]).
func CollectOutgoingBufferTrace(port naming.Named) {
	p, ok := port.(messaging.Port)
	if !ok {
		return
	}

	p.AcceptHook(&outgoingBufferHook{})
}
