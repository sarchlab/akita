package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
)

// IncomingBufferTaskKind is the Kind of the task that records how long a message
// sits in a port's incoming buffer — from the moment the connection delivers it
// until the owning component retrieves (admits) it. The task carries milestones
// for the two phases of that wait: a "reached head" milestone (emitted by the
// hook the instant the message becomes the head of the buffer) ends the
// waiting-behind-others interval, and the component adds admission milestones
// (e.g. a free slot, a free downstream port) for the time the message then
// spends at the head before it can be admitted. The component's own processing
// is the separate req_in task, which begins at retrieve.
const IncomingBufferTaskKind = "incoming_buffer"

// incomingBufferHook records, for each message delivered to a port, a buffer
// task spanning the message's residency in the incoming buffer. The task is a
// child of the message's req_out task (the request's own ID, or, for a
// response, the ID it responds to). It is located at the receiving port and
// emitted on the port's owning component, so it flows to that component's
// tracers.
//
// The task opens on HookPosPortMsgRecvd (delivery) and closes on
// HookPosPortMsgRetrieveIncoming (admission). The hook emits a "reached head"
// milestone the instant the message becomes the head of the buffer:
//   - immediately, if it is delivered into an empty buffer; or
//   - when the message ahead of it is retrieved, exposing it as the new head.
//
// Reaching the head is detected here, from the buffer's own push/pop events, so
// it is exact — a component that peeks the head only on its own tick would mark
// it late.
//
// HookPosPortMsgRecvd fires while the port holds its lock, so the hook must not
// call back into the port there. It tracks the buffer depth itself (depth) to
// tell whether a freshly delivered message is already at the head. The incoming
// buffer is FIFO and the only push/pop paths are Deliver/RetrieveIncoming, both
// of which fire these hooks, so the mirrored depth stays exact.
type incomingBufferHook struct {
	mu    sync.Mutex
	depth int // messages currently in the port's incoming buffer
}

// Func implements hooking.Hook.
func (h *incomingBufferHook) Func(ctx hooking.HookCtx) {
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
		h.onDeliver(domain, port, msg)
	case messaging.HookPosPortMsgRetrieveIncoming:
		h.onRetrieve(domain, port, msg)
	}
}

// onDeliver opens the buffer task. A message delivered into an empty buffer is
// already at the head, so its reached-head milestone fires at once (its
// waiting-behind-others interval is zero). Records nothing — and generates no
// ID — when the component is not being traced.
func (h *incomingBufferHook) onDeliver(
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

	id := MsgIDAtIncomingBuffer(msg, domain)

	h.mu.Lock()
	atHead := h.depth == 0
	h.depth++
	h.mu.Unlock()

	StartTask(domain, TaskStart{
		ID:       id,
		ParentID: parentID,
		Kind:     IncomingBufferTaskKind,
		What:     msgTypeName(msg),
		// A port holds both an incoming and an outgoing buffer, so the
		// direction qualifies the location — one location, one kind (see
		// README.md). The incoming buffer is "<port>.incoming".
		Location: port.Name() + ".incoming",
	})

	if atHead {
		h.markReachedHead(domain, port, id)
	}
}

// onRetrieve closes the retrieved message's buffer task and, if the buffer is
// not now empty, marks the message exposed as the new head as having reached
// it. The retrieve hook runs after the port releases its lock, so peeking the
// new head here is safe.
func (h *incomingBufferHook) onRetrieve(
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

	EndTask(domain, TaskEnd{ID: MsgIDAtIncomingBuffer(retrieved, domain)})
	ForgetMsgIDAtIncomingBuffer(retrieved.Meta().ID, domain)

	newHead := port.PeekIncoming()
	if newHead == nil {
		return
	}

	h.markReachedHead(domain, port, MsgIDAtIncomingBuffer(newHead, domain))
}

// markReachedHead records that a message has reached the head of the buffer and
// stopped waiting behind earlier messages. Any further delay before retrieve is
// the component's own (admission) and is recorded by the component on this same
// task.
func (h *incomingBufferHook) markReachedHead(
	domain NamedHookable, port messaging.Port, taskID uint64,
) {
	AddMilestone(domain, Milestone{
		TaskID: taskID,
		Kind:   MilestoneKindQueue,
		What:   port.Name(),
	})
}

// CollectIncomingBufferTrace attaches incoming-buffer tracing to a port. It
// accepts any named entity and is a no-op for anything that is not a
// [messaging.Port], so callers that handle entities uniformly can pass them
// through without a type switch. The emitted tasks flow to the tracers
// collecting from the port's owning component, so this is only effective once
// that component is itself being traced (see [CollectTrace]).
func CollectIncomingBufferTrace(port naming.Named) {
	p, ok := port.(messaging.Port)
	if !ok {
		return
	}

	p.AcceptHook(&incomingBufferHook{})
}
