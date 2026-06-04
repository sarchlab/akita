package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

type controlStage struct {
	ctrlPort   messaging.Port
	pipeline   *pipelineMW
	bankStages []*bankStage
}

func (s *controlStage) Tick() bool {
	madeProgress := false

	madeProgress = s.processNewRequest() || madeProgress
	madeProgress = s.processCurrentFlush() || madeProgress

	return madeProgress
}

func (s *controlStage) processCurrentFlush() bool {
	next := &s.pipeline.comp.State
	if !next.HasProcessingFlush {
		return false
	}

	if s.shouldWaitForInFlightTransactions() {
		return false
	}

	rsp := &mem.ControlRsp{Command: mem.CmdFlush, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = s.ctrlPort.AsRemote()
	rsp.Dst = next.ProcessingFlush.MsgMeta.Src
	rsp.RspTo = next.ProcessingFlush.MsgMeta.ID
	rsp.TrafficBytes = 0
	rsp.TrafficClass = "mem.ControlRsp"

	if !s.ctrlPort.CanSend() {
		return false
	}

	s.ctrlPort.Send(rsp)

	s.hardResetCache()
	next.HasProcessingFlush = false
	next.ProcessingFlush = flushReqState{}

	return true
}

func (s *controlStage) hardResetCache() {
	s.flushPort(s.pipeline.topPort)
	s.flushPort(s.pipeline.bottomPort)

	next := &s.pipeline.comp.State

	// Clear buffers directly
	next.DirBuf.Clear()
	for i := range next.BankBufs {
		next.BankBufs[i].Clear()
	}

	spec := s.pipeline.comp.Spec()
	blockSize := int(1 << spec.Log2BlockSize)
	cache.DirectoryReset(
		&next.DirectoryState,
		spec.NumSets, spec.WayAssociativity, blockSize)
	next.MSHRState = cache.MSHRState{}

	for _, bankStage := range s.pipeline.bankStages {
		bankStage.Reset()
	}

	// Clear all transactions
	next.Transactions = nil
}

func (s *controlStage) flushPort(port messaging.Port) {
	for port.PeekIncoming() != nil {
		port.RetrieveIncoming()
	}
}

// processNewRequest no longer consumes any control verb: ctrlMiddleware
// owns every verb, including the now no-op Flush. The legacy
// startCacheFlush / processCurrentFlush compound-flush path is retained
// only for its dedicated unit tests, which drive HasProcessingFlush
// directly.
func (s *controlStage) processNewRequest() bool {
	return false
}

func (s *controlStage) startCacheFlush(msg *mem.ControlReq) bool {
	next := &s.pipeline.comp.State
	if next.HasProcessingFlush {
		return false
	}

	// TODO(control-protocol): Phase 2 splits Flush from Reset; the
	// DiscardInflight/PauseAfter modifier flags were removed from
	// mem.ControlReq. flushReqState's matching fields stay as zero
	// until Phase 2 rewires this code path.
	next.HasProcessingFlush = true
	next.ProcessingFlush = flushReqState{
		MsgMeta: msg.MsgMeta,
	}

	s.ctrlPort.RetrieveIncoming()

	return true
}

func (s *controlStage) shouldWaitForInFlightTransactions() bool {
	next := &s.pipeline.comp.State
	for i := range next.Transactions {
		if !next.Transactions[i].Removed {
			return true
		}
	}
	return false
}
