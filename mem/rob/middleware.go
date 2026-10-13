package rob

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type middleware struct {
	comp *Comp
}

// Handle advances the reorder buffer by one cycle on every tick. The control
// port is serviced first so Reset or Pause can quiesce the pipeline before
// any new traffic moves. While paused the pipeline is frozen entirely. Drain
// completion is handled inside processControlMsg.
func (m *middleware) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.processControlMsg() || madeProgress

	switch m.comp.State.ControlState {
	case memcontrolprotocol.StateEnabled, memcontrolprotocol.StateDraining:
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false
	width := m.comp.Spec.NumReqPerCycle

	for i := 0; i < width; i++ {
		if !m.bottomUp() {
			break
		}
		madeProgress = true
	}

	for i := 0; i < width; i++ {
		if !m.parseBottom() {
			break
		}
		madeProgress = true
	}

	for i := 0; i < width; i++ {
		if !m.topDown() {
			break
		}
		madeProgress = true
	}

	return madeProgress
}

// topDown pulls a request from the top, forwards a shadow request to the
// bottom, and records the transaction in FIFO order. Skipped while not
// Enabled so Drain and Pause stop accepting new traffic.
//
// Admission waits belong to the incoming-buffer task. The request task starts
// only once both a transaction slot and the downstream port are available.
func (m *middleware) topDown() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}

	msg, ok := m.comp.Ports.Top.PeekIncoming()
	if !ok {
		return false
	}
	_, ok = msg.Payload.(memprotocol.AccessReq)
	req := msg
	if !ok {
		panic("rob: unsupported top-port message type")
	}

	if state.AdmissionMsgID != msg.ID {
		state.AdmissionMsgID = msg.ID
		state.WaitingForBottom = false
	}
	if len(state.Transactions) >= m.comp.Spec.BufferSize {
		if state.WaitingForBottom {
			m.comp.Ports.Top.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindNetworkBusy, m.comp.Ports.Bottom.Name()))
			state.WaitingForBottom = false
		}
		return false
	}

	// A reorder-buffer entry is free: the hardware-resource wait the request
	// spent at the head of the Top buffer is over. This admission wait belongs
	// to the incoming-buffer task; req_in (opened at retrieve, below) covers
	// only the processing that follows.
	if !state.WaitingForBottom {
		m.comp.Ports.Top.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".buffer"))
		state.WaitingForBottom = true
	}

	shadow, isRead := m.buildShadowReq(
		req, m.comp.Ports.Bottom.AsRemote(), m.comp.Spec.BottomUnit)

	if !m.comp.Ports.Bottom.CanSend() {
		return false
	}

	// The shadow request is on its way downstream: the at-head wait to send on
	// the Bottom port is over (also on the incoming-buffer task).
	m.comp.Ports.Top.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindNetworkBusy, m.comp.Ports.Bottom.Name()))

	// Admit the request: open req_in at retrieve and record the transaction.
	recvTaskID := tasks.StartReqIn(m.comp, req)
	m.tagReadWrite(req, recvTaskID)
	sendTaskID := tasks.StartReqOut(m.comp, shadow, recvTaskID)
	m.comp.Ports.Bottom.Send(shadow)

	state.Transactions = append(state.Transactions, transactionState{
		RecvTaskID:    recvTaskID,
		SendTaskID:    sendTaskID,
		ReqFromTopID:  req.ID,
		ReqFromTopSrc: req.Src,
		ReqToBottomID: shadow.ID,
		IsRead:        isRead,
	})
	m.comp.Ports.Top.RetrieveIncoming()
	state.AdmissionMsgID = 0
	state.WaitingForBottom = false

	return true
}

// parseBottom records the response that came back from the bottom unit on its
// matching transaction. Unmatched responses (e.g. left over after a flush) are
// dropped.
func (m *middleware) parseBottom() bool {
	msg, ok := m.comp.Ports.Bottom.PeekIncoming()
	if !ok {
		return false
	}

	switch content := msg.Payload.(type) {
	case memprotocol.DataReadyRsp:
		dataRsp := msg

		idx := m.findTransactionByBottomID(dataRsp.RspTo)
		m.comp.Ports.Bottom.RetrieveIncoming()

		if idx < 0 {
			return true
		}

		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		trans.RspData = content.Data
		// The bottom unit returned the data this transaction was waiting on.

		tasks.AddMilestone(m.comp, tasks.Milestone{
			TaskID: trans.RecvTaskID,
			Kind:   tasks.MilestoneKindData,
			What:   m.comp.Ports.Bottom.Name(),
		})
		tasks.EndReqOut(m.comp, trans.SendTaskID)
		return true
	case memprotocol.WriteDoneRsp:
		dataRsp := msg

		idx := m.findTransactionByBottomID(dataRsp.RspTo)
		m.comp.Ports.Bottom.RetrieveIncoming()

		if idx < 0 {
			return true
		}

		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		// The bottom unit acknowledged the write subtask this transaction
		// dispatched.
		tasks.AddMilestone(m.comp, tasks.Milestone{
			TaskID: trans.RecvTaskID,
			Kind:   tasks.MilestoneKindSubTask,
			What:   m.comp.Ports.Bottom.Name(),
		})
		tasks.EndReqOut(m.comp, trans.SendTaskID)
		return true
	default:
		m.comp.Ports.Bottom.RetrieveIncoming()
		return true
	}
}

// bottomUp releases the head-of-line transaction once its bottom-unit response
// has arrived, forwarding a response to the original requester.
func (m *middleware) bottomUp() bool {
	state := &m.comp.State
	if len(state.Transactions) == 0 {
		return false
	}

	head := state.Transactions[0]
	if !head.HasRsp {
		return false
	}

	// The head transaction has its response and may now retire. The interval
	// since its bottom response arrived is the time it spent blocked behind
	// older, not-yet-retired transactions (in-order commit). Emitted before the
	// CanSend retry and the response-sent milestone so this more meaningful
	// reason wins a same-tick tie; the (Kind, What) dedup keeps the first
	// emission across retries and a zero-length head-of-line wait collapses.
	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: head.RecvTaskID,
		Kind:   tasks.MilestoneKindDependency,
		What:   m.comp.Name() + ".reorder",
	})

	rsp := m.buildTopRsp(head, m.comp.Ports.Top.AsRemote())

	if !m.comp.Ports.Top.CanSend() {
		return false
	}

	m.comp.Ports.Top.Send(rsp)

	state.Transactions = state.Transactions[1:]

	// The response is on its way to the requester: the wait to send on the Top
	// port is over.
	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: head.RecvTaskID,
		Kind:   tasks.MilestoneKindNetworkBusy,
		What:   m.comp.Ports.Top.Name(),
	})

	tasks.EndReqIn(m.comp, head.RecvTaskID)

	return true
}

func (m *middleware) findTransactionByBottomID(id uint64) int {
	for i := range m.comp.State.Transactions {
		if m.comp.State.Transactions[i].ReqToBottomID == id {
			return i
		}
	}
	return -1
}

// buildShadowReq mirrors the incoming request as a fresh request the bottom
// unit will see. The returned bool is true when the source request is a read.
func (m *middleware) buildShadowReq(
	req messaging.Msg, src, dst messaging.RemotePort,
) (messaging.Msg, bool) {
	switch content := req.Payload.(type) {
	case memprotocol.ReadReq:
		r := req

		shadow := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        content.Address,
			AccessByteSize: content.AccessByteSize,
			PID:            content.PID,
		},
			ID:           m.comp.NewID(),
			Src:          src,
			Dst:          dst,
			TrafficBytes: r.TrafficBytes,
			TrafficClass: r.TrafficClass}

		return shadow, true
	case memprotocol.WriteReq:
		r := req

		shadow := messaging.Msg{Payload: memprotocol.WriteReq{
			Address:   content.Address,
			Data:      content.Data,
			DirtyMask: content.DirtyMask,
			PID:       content.PID,
		},
			ID:           m.comp.NewID(),
			Src:          src,
			Dst:          dst,
			TrafficBytes: r.TrafficBytes,
			TrafficClass: r.TrafficClass}

		return shadow, false
	default:
		panic("rob: unsupported request type")
	}
}

func (m *middleware) buildTopRsp(
	trans transactionState, src messaging.RemotePort,
) messaging.Msg {
	if trans.IsRead {
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: trans.RspData},
			ID:           m.comp.NewID(),
			Src:          src,
			Dst:          trans.ReqFromTopSrc,
			RspTo:        trans.ReqFromTopID,
			TrafficBytes: len(trans.RspData) + 4,
			TrafficClass: "memprotocol.DataReadyRsp"}

		return rsp
	}

	rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
		ID:           m.comp.NewID(),
		Src:          src,
		Dst:          trans.ReqFromTopSrc,
		RspTo:        trans.ReqFromTopID,
		TrafficBytes: 4,
		TrafficClass: "memprotocol.WriteDoneRsp"}

	return rsp
}

// tagReadWrite labels the req_in task as a "read" or a "write" so traces can be
// filtered by access type. Called once, when the request is admitted (retrieved)
// and its req_in task opens.
func (m *middleware) tagReadWrite(req messaging.Msg, taskID uint64) {
	what := "write"
	if _, ok := req.Payload.(memprotocol.ReadReq); ok {
		what = "read"
	}

	tasks.AddTag(m.comp, tasks.Tag{
		TaskID: taskID,
		What:   what,
	})
}

// processControlMsg handles the universal control verbs and finalizes
// a pending Drain when in-flight transactions have settled.
func (m *middleware) processControlMsg() bool {
	if m.completePendingDrain() {
		return true
	}

	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not dequeued — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState == memcontrolprotocol.StateDraining {
		return false
	}

	msg, ok := m.comp.Ports.Control.PeekIncoming()
	if !ok {
		return false
	}
	_, ok = msg.Payload.(memcontrolprotocol.Req)
	req := msg
	if !ok {
		m.comp.Ports.Control.RetrieveIncoming()
		return true
	}

	switch req.Payload.(memcontrolprotocol.Req).Command {
	case memcontrolprotocol.CmdPause:
		return m.handlePause(req)
	case memcontrolprotocol.CmdDrain:
		return m.handleDrain(req)
	case memcontrolprotocol.CmdEnable:
		return m.handleEnable(req)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(req)
	default:
		return m.handleUnsupported(req)
	}
}

// completePendingDrain notices Drain has reached quiescence (no
// in-flight transactions) and acks. The component transitions to
// Paused.
func (m *middleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateDraining {
		return false
	}
	if len(state.Transactions) != 0 {
		return false
	}
	if !m.comp.Ports.Control.CanSend() {
		return false
	}

	m.comp.Ports.Control.Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = memcontrolprotocol.StatePaused
	return true
}

func (m *middleware) handlePause(req messaging.Msg) bool {
	if !m.comp.Ports.Control.CanSend() {
		return false
	}
	m.comp.State.ControlState = memcontrolprotocol.StatePaused
	m.comp.Ports.Control.Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.comp.Ports.Control.RetrieveIncoming()
	return true
}

func (m *middleware) handleDrain(req messaging.Msg) bool {
	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.comp.Ports.Control.RetrieveIncoming()
	return true
}

func (m *middleware) handleEnable(req messaging.Msg) bool {
	if !m.comp.Ports.Control.CanSend() {
		return false
	}

	m.comp.Ports.Control.Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))

	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateEnabled

	// Enable resumes from Paused; it must not discard traffic queued while
	// paused (e.g. bottom responses that retire frozen in-flight
	// transactions). They are processed once the pipeline runs again.
	m.comp.Ports.Control.RetrieveIncoming()
	return true
}

// handleReset drops every in-flight transaction, drains stale port
// traffic, and lands the ROB back in Enabled. The req_in/req_out tracing tasks
// that topDown opened for each in-flight transaction are ended (and their
// owned task IDs discarded) so they do not outlive the transactions.
func (m *middleware) handleReset(req messaging.Msg) bool {
	if !m.comp.Ports.Control.CanSend() {
		return false
	}

	m.comp.Ports.Control.Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))

	state := &m.comp.State
	m.endInflightTasks()
	state.Transactions = state.Transactions[:0]
	state.AdmissionMsgID = 0
	state.WaitingForBottom = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = memcontrolprotocol.StateEnabled

	// Buffer tasks for messages still queued on the Top/Bottom ports are closed
	// by the drains below: each RetrieveIncoming ends the buffer task. No pre-admission req_in exists to clean up — req_in
	// now opens only at retrieve.
	drainIncoming(m.comp.Ports.Top)
	drainIncoming(m.comp.Ports.Bottom)

	m.comp.Ports.Control.RetrieveIncoming()
	return true
}

func (m *middleware) handleUnsupported(req messaging.Msg) bool {
	if !m.comp.Ports.Control.CanSend() {
		return false
	}
	m.comp.Ports.Control.Send(makeCtrlRsp(m.comp, req.Payload.(memcontrolprotocol.Req).Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.comp.Ports.Control.RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	c *Comp,
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) messaging.Msg {
	rsp := messaging.Msg{Payload: memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	},
		ID:           c.NewID(),
		Src:          c.Ports.Control.AsRemote(),
		Dst:          dst,
		RspTo:        rspTo,
		TrafficClass: "memcontrolprotocol.Rsp"}

	return rsp
}

func drainIncoming(p messaging.Port) {
	for {
		if _, ok := p.RetrieveIncoming(); !ok {
			break
		}
	}
}

// endInflightTasks completes the top-side req_in tracing task and finalizes the
// shadow req_out task for each in-flight transaction, so a control path that
// drops the transaction table leaves no started-never-ended task. topDown opens both tasks together when it admits a
// request, so both are open until the transaction retires; the end is
// idempotent if the shadow's response already finalized its req_out. Mirrors
// bottomUp (req_in) and parseBottom (req_out) completion.
func (m *middleware) endInflightTasks() {
	for _, trans := range m.comp.State.Transactions {
		tasks.EndReqIn(m.comp, trans.RecvTaskID)
		tasks.EndReqOut(m.comp, trans.SendTaskID)
	}
}

var _ modeling.Middleware = (*middleware)(nil)
