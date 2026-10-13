package taskcore

import "github.com/sarchlab/akita/v5/sim/messaging"

// requestParents resolves shared message links under the tracker lock. Missing
// sender records leave ParentID zero (for example after a tracking-off checkpoint).
type requestParents struct {
	byMessage map[uint64]uint64
	byTask    map[uint64]uint64
}

// ResolveStart records req_out/flit_e2e identity or resolves a task parent.
// Call for every start before filtering or retaining the event.
func (p *requestParents) ResolveStart(task Task) Task {
	if task.MsgID == 0 {
		return task
	}
	switch task.Kind {
	case KindReqOut, KindFlitTransport:
		if p.byMessage == nil {
			p.byMessage = make(map[uint64]uint64)
			p.byTask = make(map[uint64]uint64)
		}
		p.byMessage[task.MsgID] = task.ID
		p.byTask[task.ID] = task.MsgID
	case KindReqIn:
		task.ParentID = p.byMessage[task.MsgID]
	case KindIncomingBuffer, KindOutgoingBuffer, KindMsgTransport:
		msgID := task.MsgID
		if msg, ok := task.Detail.(messaging.Msg); ok && msg.IsRsp() {
			msgID = msg.RspTo
		}
		task.ParentID = p.byMessage[msgID]
	}
	return task
}

// End forgets a completed req_out or flit_e2e. Call for every end, including tasks
// omitted by the observer's recording filter.
func (p *requestParents) End(taskID uint64) {
	msgID, ok := p.byTask[taskID]
	if !ok {
		return
	}
	if p.byMessage[msgID] == taskID {
		delete(p.byMessage, msgID)
	}
	delete(p.byTask, taskID)
}
