package tracing

import "github.com/sarchlab/akita/v5/sim/messaging"

// RequestTaskParents resolves request, buffer, and message-transport parents for one tracer. Its
// zero value is ready for use. The caller must serialize access (normally with
// the tracer lock). It is never checkpointed or consulted by simulation code. A missing
// sender (untraced or already in flight at restore) leaves ParentID zero.
type RequestTaskParents struct {
	byMessage map[uint64]uint64
	byTask    map[uint64]uint64
}

// ResolveStart records req_out identity or resolves a request/buffer/transport parent.
// Call for every start before filtering or retaining the event.
func (p *RequestTaskParents) ResolveStart(task TaskStart) TaskStart {
	if task.MsgID == 0 {
		return task
	}
	switch task.Kind {
	case ReqOutTaskKind:
		if p.byMessage == nil {
			p.byMessage = make(map[uint64]uint64)
			p.byTask = make(map[uint64]uint64)
		}
		p.byMessage[task.MsgID] = task.ID
		p.byTask[task.ID] = task.MsgID
	case ReqInTaskKind:
		task.ParentID = p.byMessage[task.MsgID]
	case IncomingBufferTaskKind, OutgoingBufferTaskKind, "msg_e2e":
		msgID := task.MsgID
		if msg, ok := task.Detail.(messaging.Msg); ok && msg.IsRsp() {
			msgID = msg.RspTo
		}
		task.ParentID = p.byMessage[msgID]
	}
	return task
}

// End forgets a completed req_out. Call for every task end, including tasks
// omitted by the observer's recording filter.
func (p *RequestTaskParents) End(taskID uint64) {
	msgID, ok := p.byTask[taskID]
	if !ok {
		return
	}
	if p.byMessage[msgID] == taskID {
		delete(p.byMessage, msgID)
	}
	delete(p.byTask, taskID)
}
