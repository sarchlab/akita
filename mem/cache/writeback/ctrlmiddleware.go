package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ctrlMiddleware owns every control verb except CmdFlush, which is
// handled by the legacy flusher pipeline. The verbs it owns are
// Pause, Drain, Enable, Reset, Invalidate. CmdFlush is left in the
// Control port's incoming queue for the flusher to pick up.
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

// completePendingDrain finalizes a pending Drain once no transactions
// and no MSHR-stage work remain.
func (m *ctrlMiddleware) completePendingDrain() bool {
	next := &m.pipeline.comp.State
	if cacheState(next.CacheState) != cacheStateDraining {
		return false
	}

	if !cacheIsQuiescent(next) {
		return false
	}

	if !m.ctrlPort.CanSend() {
		return false
	}

	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdDrain,
		next.CurrentCmdSrc, next.CurrentCmdID, true, ""))
	next.CacheState = int(cacheStatePaused)
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
	return true
}

func cacheIsQuiescent(state *State) bool {
	if len(state.Transactions) != 0 {
		return false
	}
	if state.WriteBufferBuf.Size() > 0 {
		return false
	}
	for _, c := range state.BankInflightTransCounts {
		if c > 0 {
			return false
		}
	}
	for _, c := range state.BankDownwardInflightTransCounts {
		if c > 0 {
			return false
		}
	}
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
		// Owned by flusher; leave in queue.
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
	m.pipeline.comp.State.CacheState = int(cacheStatePaused)
	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort.RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req *mem.ControlReq) bool {
	next := &m.pipeline.comp.State
	next.CacheState = int(cacheStateDraining)
	next.CurrentCmdID = req.ID
	next.CurrentCmdSrc = req.Src
	m.ctrlPort.RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req *mem.ControlReq) bool {
	if !m.ctrlPort.CanSend() {
		return false
	}
	clearPort(m.pipeline.topPort)
	clearPort(m.pipeline.bottomPort)
	m.pipeline.comp.State.CacheState = int(cacheStateRunning)
	m.ctrlPort.Send(makeCtrlRsp(m.ctrlPort, mem.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort.RetrieveIncoming()
	return true
}

// handleReset wipes the cache back to a freshly-built shape: empty
// directory, empty MSHR, empty buffers, empty in-flight transactions,
// empty port queues. No writeback happens — dirty data is discarded
// per the resolved-decision policy ("Invalidate-on-dirty: drop
// silently" generalized to Reset).
func (m *ctrlMiddleware) handleReset(req *mem.ControlReq) bool {
	if !m.ctrlPort.CanSend() {
		return false
	}

	next := &m.pipeline.comp.State
	spec := m.pipeline.comp.Spec()
	blockSize := 1 << spec.Log2BlockSize
	cache.DirectoryReset(
		&next.DirectoryState, spec.NumSets, spec.WayAssociativity, blockSize)
	next.MSHRState = cache.MSHRState{}
	next.Transactions = nil
	next.EvictingList = map[uint64]bool{}
	next.DirStageBuf.Clear()
	for i := range next.DirToBankBufs {
		next.DirToBankBufs[i].Clear()
	}
	for i := range next.WriteBufferToBankBufs {
		next.WriteBufferToBankBufs[i].Clear()
	}
	next.MSHRStageBuf.Clear()
	next.WriteBufferBuf.Clear()
	next.DirPipeline.Clear()
	next.DirPostPipelineBuf.Clear()
	for i := range next.BankPipelines {
		next.BankPipelines[i].Clear()
	}
	for i := range next.BankPostPipelineBufs {
		next.BankPostPipelineBufs[i].Clear()
	}
	for i := range next.BankInflightTransCounts {
		next.BankInflightTransCounts[i] = 0
	}
	for i := range next.BankDownwardInflightTransCounts {
		next.BankDownwardInflightTransCounts[i] = 0
	}
	next.PendingEvictionIndices = nil
	next.InflightFetchIndices = nil
	next.InflightEvictionIndices = nil
	next.HasProcessingMSHREntry = false
	next.ProcessingMSHREntryIdx = 0
	next.FlusherBlockToEvictRefs = nil
	next.HasProcessingFlush = false
	next.ProcessingFlush = flushReqState{}
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
	next.CacheState = int(cacheStateRunning)

	clearPort(m.pipeline.topPort)
	clearPort(m.pipeline.bottomPort)

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
