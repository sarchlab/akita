package pagetablemmu

import (
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

func (m *walkMiddleware) respond() bool {
	c := m.c
	progress := false
	for range c.Spec.AdmissionWidth {
		did := false
		for n := range c.State.Walks {
			i := (c.State.NextResponse + n) % len(c.State.Walks)
			w := &c.State.Walks[i]
			if w.Stage == walkDone && m.replyOrForward(w) {
				c.State.NextResponse = (i + 1) % len(c.State.Walks)
				did = true
				break
			}
		}
		if !did {
			break
		}
		progress = true
	}
	return progress
}

func (m *walkMiddleware) replyOrForward(w *walk) bool {
	for i, wait := range w.Waiters {
		if w.Page.Valid && (!m.c.Spec.ConsultRemote || w.Page.DeviceID == wait.Device) {
			if m.reply(w, i, pagefaultprotocol.Rsp{Result: pagefaultprotocol.Resolved, Mapping: w.Page}) {
				return true
			}
			continue
		}
		d := slices.IndexFunc(w.Decisions, func(d decision) bool { return d.Device == wait.Device })
		if d < 0 {
			if m.forward(w, wait) {
				return true
			}
			continue
		}
		if w.Decisions[d].Response.Result != 0 && m.reply(w, i, w.Decisions[d].Response) {
			return true
		}
	}
	return false
}

func (m *walkMiddleware) forward(w *walk, wait waiter) bool {
	c := m.c
	p := c.Ports.PageFault
	if !p.CanSend() {
		return false
	}
	req := messaging.Msg{ID: c.NewID(), Src: p.AsRemote(), Dst: c.Spec.FaultHandler, Payload: pagefaultprotocol.Req{
		PID: w.PID, VAddr: wait.VA, DeviceID: wait.Device, HasMapping: w.Page.Valid, Mapping: w.Page}}
	task := c.TaskTracker().StartReqOut(c, req, wait.Task)
	w.markDecision(c, wait.Device, tasks.MilestoneKindHardwareResource)
	w.Decisions = append(w.Decisions, decision{Device: wait.Device, ID: req.ID, Task: task})
	p.Send(req)
	return true
}

func (m *walkMiddleware) receiveDecision() bool {
	c := m.c
	msg, ok := c.Ports.PageFault.RetrieveIncoming()
	if !ok {
		return false
	}
	rsp := msg.Payload.(pagefaultprotocol.Rsp)
	for i := range c.State.Walks {
		w := &c.State.Walks[i]
		for j := range w.Decisions {
			d := &w.Decisions[j]
			if d.ID != msg.RspTo || d.Response.Result != 0 {
				continue
			}
			switch rsp.Result {
			case pagefaultprotocol.Resolved:
				p := rsp.Mapping
				if !p.Valid || p.PID != w.PID || p.VAddr != w.VPN<<c.Spec.Log2PageSize ||
					p.PageSize != c.Spec.pageSize() || p.PAddr%p.PageSize != 0 {
					panic("pagetablemmu: resolved fault has invalid mapping")
				}
			case pagefaultprotocol.Blocked:
			default:
				panic("pagetablemmu: invalid fault result")
			}
			d.Response = rsp
			w.markDecision(c, d.Device, tasks.MilestoneKindTranslation)
			c.TaskTracker().EndReqOut(d.Task)
			return true
		}
	}
	panic("pagetablemmu: unmatched fault response")
}

func (m *walkMiddleware) reply(w *walk, i int, answer pagefaultprotocol.Rsp) bool {
	c := m.c
	wait := w.Waiters[i]
	p := c.Ports.Top
	page := answer.Mapping
	if answer.Result == pagefaultprotocol.Blocked {
		// Existing TLBs identify the MSHR to drain by the returned PID and page VA.
		page = w.Page
		page.PID, page.VAddr, page.PageSize = w.PID, w.VPN<<c.Spec.Log2PageSize, c.Spec.pageSize()
	}
	var payload any = addrtransprotocol.Rsp{Page: page, Fault: answer.Result == pagefaultprotocol.Blocked}
	if wait.FaultIn {
		p = c.Ports.FaultIn
		payload = answer
	}
	if !p.CanSend() {
		return false
	}
	p.Send(messaging.Msg{ID: c.NewID(), RspTo: wait.ID, Src: p.AsRemote(), Dst: wait.Src, Payload: payload})
	c.TaskTracker().EndReqIn(wait.Task)
	w.Waiters = slices.Delete(w.Waiters, i, i+1)
	if len(w.Waiters) == 0 {
		*w = walk{}
		return true
	}
	if !slices.ContainsFunc(w.Waiters, func(v waiter) bool { return v.Device == wait.Device }) {
		w.Decisions = slices.DeleteFunc(w.Decisions, func(d decision) bool { return d.Device == wait.Device })
	}
	return true
}
