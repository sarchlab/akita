package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ctrlMiddleware owns Pause, Drain, Enable, Reset, and the
// unsupported-Invalidate response. CmdFlush is owned by controlStage
// (legacy compound implementation).
type ctrlMiddleware struct {
	pipeline *pipelineMW
	ctrlPort messaging.Port
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	madeProgress = m.handleIncoming() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) completePendingDrain() bool {
	next := &m.pipeline.comp.State
	if !next.IsDraining {
		return false
	}

	for i := range next.Transactions {
		if !next.Transactions[i].Removed {
			return false
		}
	}

	if !m.ctrlPort.CanSend() {
		return false
	}

	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdDrain,
		next.CurrentCmdSrc, next.CurrentCmdID, true, ""))
	next.IsDraining = false
	next.IsPaused = true
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
	return true
}

func (m *ctrlMiddleware) handleIncoming() bool {
	msg := m.ctrlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(*mem.ControlReq)
	if !ok {
		return false
	}

	switch req.Command {
	case mem.CmdPause:
		return m.handlePause(req)
	case mem.CmdDrain:
		return m.handleDrain(req)
	case mem.CmdEnable:
		return m.handleEnable(req)
	case mem.CmdReset:
		return m.handleReset(req)
	case mem.CmdFlush:
		// Owned by controlStage; leave in queue.
		return false
	case mem.CmdInvalidate:
		return m.handleUnsupported(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req *mem.ControlReq) bool {
	if !m.ctrlPort.CanSend() {
		return false
	}
	m.pipeline.comp.State.IsPaused = true
	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort.RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req *mem.ControlReq) bool {
	next := &m.pipeline.comp.State
	next.IsDraining = true
	next.CurrentCmdID = req.ID
	next.CurrentCmdSrc = req.Src
	m.ctrlPort.RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req *mem.ControlReq) bool {
	if !m.ctrlPort.CanSend() {
		return false
	}

	next := &m.pipeline.comp.State
	next.IsPaused = false
	next.IsDraining = false

	for m.pipeline.topPort.PeekIncoming() != nil {
		m.pipeline.topPort.RetrieveIncoming()
	}
	for m.pipeline.bottomPort.PeekIncoming() != nil {
		m.pipeline.bottomPort.RetrieveIncoming()
	}

	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort.RetrieveIncoming()
	return true
}

// handleReset wipes the cache back to a freshly-built state without
// writeback (the writethrough cache holds no dirty data anyway).
func (m *ctrlMiddleware) handleReset(req *mem.ControlReq) bool {
	if !m.ctrlPort.CanSend() {
		return false
	}

	next := &m.pipeline.comp.State
	spec := m.pipeline.comp.Spec()

	next.DirBuf.Clear()
	for i := range next.BankBufs {
		next.BankBufs[i].Clear()
	}
	cache.DirectoryReset(
		&next.DirectoryState, spec.NumSets, spec.WayAssociativity,
		int(1<<spec.Log2BlockSize))
	next.MSHRState = cache.MSHRState{}
	next.Transactions = nil
	next.HasProcessingFlush = false
	next.ProcessingFlush = flushReqState{}
	next.IsPaused = false
	next.IsDraining = false
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""

	for m.pipeline.topPort.PeekIncoming() != nil {
		m.pipeline.topPort.RetrieveIncoming()
	}
	for m.pipeline.bottomPort.PeekIncoming() != nil {
		m.pipeline.bottomPort.RetrieveIncoming()
	}

	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort.RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(req *mem.ControlReq) bool {
	if !m.ctrlPort.CanSend() {
		return false
	}
	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, req.Command,
		req.Src, req.ID, false, control.ErrUnsupported))
	m.ctrlPort.RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd mem.ControlCommand,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) *mem.ControlRsp {
	rsp := &mem.ControlRsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "mem.ControlRsp"
	return rsp
}
