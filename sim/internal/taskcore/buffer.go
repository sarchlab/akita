package taskcore

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

type bufferKey struct {
	port     messaging.Port
	msgID    uint64
	incoming bool
}
type bufferHook struct{ tracker *Tracker }

// RegisterPort attaches the task service's generic port hook during assembly.
// The port remains independent of tasks, including its checkpoint format.
func RegisterPort(t *Tracker, p messaging.Port) {
	if t == nil {
		return
	}
	o, ok := p.Owner().(Owner)
	if !ok || o.TaskTracker() != t {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.frozen {
		panic("tasks: ports must be registered before running")
	}
	if t.ports[p] {
		return
	}
	t.ports[p] = true
	p.AcceptHook(&bufferHook{tracker: t})
}

func (h *bufferHook) Func(ctx hooking.HookCtx) {
	incoming, enqueue := false, false
	switch ctx.Pos {
	case messaging.HookPosPortMsgRecvd:
		incoming, enqueue = true, true
	case messaging.HookPosPortMsgSend:
		enqueue = true
	case messaging.HookPosPortMsgRetrieveIncoming:
		incoming = true
	case messaging.HookPosPortMsgRetrieveOutgoing:
	default:
		return
	}
	p := ctx.Domain.(messaging.Port)
	o := p.Owner().(Owner)
	msg := ctx.Item.(messaging.Msg)
	change := ctx.Detail.(messaging.PortBufferChange)
	t := h.tracker
	t.mu.Lock()
	defer t.mu.Unlock()
	key := bufferKey{p, msg.ID, incoming}
	if enqueue {
		kind, direction := KindOutgoingBuffer, ".outgoing"
		if incoming {
			kind, direction = KindIncomingBuffer, ".incoming"
		}
		t.buffers[key] = t.start(o, StartInfo{MsgID: msg.ID, Kind: kind, What: msgTypeName(msg),
			Location: p.Name() + direction, Detail: msg})
	} else {
		t.end(o, t.buffers[key])
		delete(t.buffers, key)
	}
	if change.HeadChanged && change.HasHead {
		id := t.buffers[bufferKey{p, change.Head.ID, incoming}]
		t.milestone(o, Milestone{TaskID: id, Kind: MilestoneKindQueue, What: p.Name()})
	}
}

// AddBufferMilestone resolves an incoming-buffer task by message identity. Call
// before RetrieveIncoming. Unknown messages (including preexisting/restored
// buffer contents) are ignored, never attributed to a later tracked message.
func AddBufferMilestone(o Owner, p messaging.Port, msg messaging.Msg, m Milestone) {
	t := o.TaskTracker()
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m.TaskID = t.buffers[bufferKey{p, msg.ID, true}]
	t.milestone(o, m)
}
