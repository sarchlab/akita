package rob

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type middleware struct {
	comp *Comp
}

func (m *middleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *middleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *middleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

// Tick advances the reorder buffer by one cycle. The control port is serviced
// first so a flush can quiesce the pipeline before any new traffic moves.
func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.processControlMsg() || madeProgress

	if !m.comp.State.IsFlushing {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false
	width := m.comp.Spec().NumReqPerCycle

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
// bottom, and records the transaction in FIFO order.
func (m *middleware) topDown() bool {
	state := &m.comp.State
	if len(state.Transactions) >= m.comp.Spec().BufferSize {
		return false
	}

	msg := m.topPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(mem.AccessReq)
	if !ok {
		panic("rob: unsupported top-port message type")
	}

	shadow, isRead := m.buildShadowReq(
		req, m.bottomPort().AsRemote(), m.comp.Spec().BottomUnit)

	if !m.bottomPort().CanSend() {
		return false
	}

	m.bottomPort().Send(shadow)

	state.Transactions = append(state.Transactions, transactionState{
		ReqFromTopID:  req.Meta().ID,
		ReqFromTopSrc: req.Meta().Src,
		ReqToBottomID: shadow.Meta().ID,
		IsRead:        isRead,
	})
	m.topPort().RetrieveIncoming()

	tracing.TraceReqReceive(req, m.comp)
	tracing.TraceReqInitiate(shadow, m.comp,
		tracing.MsgIDAtReceiver(req, m.comp))

	return true
}

// parseBottom records the response that came back from the bottom unit on its
// matching transaction. Unmatched responses (e.g. left over after a flush) are
// dropped.
func (m *middleware) parseBottom() bool {
	msg := m.bottomPort().PeekIncoming()
	if msg == nil {
		return false
	}

	switch dataRsp := msg.(type) {
	case mem.DataReadyRsp:
		idx := m.findTransactionByBottomID(dataRsp.RspTo)
		m.bottomPort().RetrieveIncoming()

		if idx < 0 {
			return true
		}

		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		trans.RspData = dataRsp.Data
		tracing.TraceReqFinalize(m.shadowReqTraceMsg(*trans), m.comp)
		return true
	case mem.WriteDoneRsp:
		idx := m.findTransactionByBottomID(dataRsp.RspTo)
		m.bottomPort().RetrieveIncoming()

		if idx < 0 {
			return true
		}

		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		tracing.TraceReqFinalize(m.shadowReqTraceMsg(*trans), m.comp)
		return true
	default:
		m.bottomPort().RetrieveIncoming()
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

	rsp := m.buildTopRsp(head, m.topPort().AsRemote())

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	state.Transactions = state.Transactions[1:]

	tracing.TraceReqComplete(m.topReqTraceMsg(head), m.comp)

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
	req mem.AccessReq, src, dst messaging.RemotePort,
) (mem.AccessReq, bool) {
	switch r := req.(type) {
	case mem.ReadReq:
		shadow := mem.ReadReq{
			Address:        r.Address,
			AccessByteSize: r.AccessByteSize,
			PID:            r.PID,
		}
		shadow.ID = timing.GetIDGenerator().Generate()
		shadow.Src = src
		shadow.Dst = dst
		shadow.TrafficBytes = r.TrafficBytes
		shadow.TrafficClass = r.TrafficClass
		return shadow, true
	case mem.WriteReq:
		shadow := mem.WriteReq{
			Address:   r.Address,
			Data:      r.Data,
			DirtyMask: r.DirtyMask,
			PID:       r.PID,
		}
		shadow.ID = timing.GetIDGenerator().Generate()
		shadow.Src = src
		shadow.Dst = dst
		shadow.TrafficBytes = r.TrafficBytes
		shadow.TrafficClass = r.TrafficClass
		return shadow, false
	default:
		panic("rob: unsupported request type")
	}
}

func (m *middleware) buildTopRsp(
	trans transactionState, src messaging.RemotePort,
) messaging.Msg {
	if trans.IsRead {
		rsp := mem.DataReadyRsp{Data: trans.RspData}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = src
		rsp.Dst = trans.ReqFromTopSrc
		rsp.RspTo = trans.ReqFromTopID
		rsp.TrafficBytes = len(trans.RspData) + 4
		rsp.TrafficClass = "mem.DataReadyRsp"
		return rsp
	}

	rsp := mem.WriteDoneRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = src
	rsp.Dst = trans.ReqFromTopSrc
	rsp.RspTo = trans.ReqFromTopID
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.WriteDoneRsp"
	return rsp
}

// shadowReqTraceMsg reconstructs a minimal request to use as the subject of a
// trace event for the shadow request the reorder buffer issued.
func (m *middleware) shadowReqTraceMsg(trans transactionState) messaging.Msg {
	if trans.IsRead {
		req := mem.ReadReq{}
		req.ID = trans.ReqToBottomID
		req.Src = m.bottomPort().AsRemote()
		req.Dst = m.comp.Spec().BottomUnit
		return req
	}
	req := mem.WriteReq{}
	req.ID = trans.ReqToBottomID
	req.Src = m.bottomPort().AsRemote()
	req.Dst = m.comp.Spec().BottomUnit
	return req
}

// topReqTraceMsg reconstructs a minimal request to use as the subject of a
// trace event for the original top-side request.
func (m *middleware) topReqTraceMsg(trans transactionState) messaging.Msg {
	if trans.IsRead {
		req := mem.ReadReq{}
		req.ID = trans.ReqFromTopID
		req.Src = trans.ReqFromTopSrc
		req.Dst = m.topPort().AsRemote()
		return req
	}
	req := mem.WriteReq{}
	req.ID = trans.ReqFromTopID
	req.Src = trans.ReqFromTopSrc
	req.Dst = m.topPort().AsRemote()
	return req
}

// processControlMsg handles the two control commands the reorder buffer
// understands: a flush that drops in-flight transactions and quiesces the
// pipeline, and an enable that drains stale port traffic and resumes work.
func (m *middleware) processControlMsg() bool {
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(mem.ControlReq)
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Command {
	case mem.CmdFlush:
		return m.handleFlush(req)
	case mem.CmdEnable:
		return m.handleEnable(req)
	default:
		m.ctrlPort().RetrieveIncoming()
		return true
	}
}

func (m *middleware) handleFlush(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	rsp := mem.ControlRsp{Command: mem.CmdFlush, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = req.Src
	rsp.RspTo = req.ID
	rsp.TrafficClass = "mem.ControlRsp"

	m.ctrlPort().Send(rsp)

	state := &m.comp.State
	state.Transactions = state.Transactions[:0]
	state.IsFlushing = true

	m.ctrlPort().RetrieveIncoming()

	return true
}

func (m *middleware) handleEnable(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	rsp := mem.ControlRsp{Command: mem.CmdEnable, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = req.Src
	rsp.RspTo = req.ID
	rsp.TrafficClass = "mem.ControlRsp"

	m.ctrlPort().Send(rsp)

	state := &m.comp.State
	state.Transactions = state.Transactions[:0]
	state.IsFlushing = false

	drainIncoming(m.topPort())
	drainIncoming(m.bottomPort())

	m.ctrlPort().RetrieveIncoming()

	return true
}

func drainIncoming(p messaging.Port) {
	for p.RetrieveIncoming() != nil {
	}
}

var _ modeling.Middleware = (*middleware)(nil)
