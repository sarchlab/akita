package writethroughcache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim"
)

type controlStage struct {
	ctrlPort   sim.Port
	pipeline   *pipelineMW
	bankStages []*bankStage

	currFlushReq *cache.FlushReq
}

func (s *controlStage) Tick() bool {
	madeProgress := false

	madeProgress = s.processNewRequest() || madeProgress
	madeProgress = s.processCurrentFlush() || madeProgress

	return madeProgress
}

func (s *controlStage) processCurrentFlush() bool {
	if s.currFlushReq == nil {
		return false
	}

	if s.shouldWaitForInFlightTransactions() {
		return false
	}

	rsp := &cache.FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = s.ctrlPort.AsRemote()
	rsp.Dst = s.currFlushReq.Src
	rsp.RspTo = s.currFlushReq.ID
	rsp.TrafficBytes = 0
	rsp.TrafficClass = "ctrl-rsp"

	err := s.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	s.hardResetCache()
	s.currFlushReq = nil

	return true
}

func (s *controlStage) hardResetCache() {
	s.flushPort(s.pipeline.topPort)
	s.flushPort(s.pipeline.bottomPort)

	next := s.pipeline.comp.GetNextState()

	// Clear buffers directly
	next.DirBuf.Clear()
	for i := range next.BankBufs {
		next.BankBufs[i].Clear()
	}

	spec := s.pipeline.GetSpec()
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

	if s.currFlushReq.PauseAfterFlushing {
		next.IsPaused = true
	}
}

func (s *controlStage) flushPort(port sim.Port) {
	for port.PeekIncoming() != nil {
		port.RetrieveIncoming()
	}
}

func (s *controlStage) processNewRequest() bool {
	msgI := s.ctrlPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	switch msg := msgI.(type) {
	case *cache.FlushReq:
		return s.startCacheFlush(msg)
	case *cache.RestartReq:
		return s.doCacheRestart(msg)
	default:
		log.Panicf("cannot handle request of type %s ",
			reflect.TypeOf(msgI))
	}

	panic("never")
}

func (s *controlStage) startCacheFlush(msg *cache.FlushReq) bool {
	if s.currFlushReq != nil {
		return false
	}

	s.currFlushReq = msg
	s.ctrlPort.RetrieveIncoming()

	return true
}

func (s *controlStage) doCacheRestart(msg *cache.RestartReq) bool {
	next := s.pipeline.comp.GetNextState()
	next.IsPaused = false

	s.ctrlPort.RetrieveIncoming()

	for s.pipeline.topPort.PeekIncoming() != nil {
		s.pipeline.topPort.RetrieveIncoming()
	}

	for s.pipeline.bottomPort.PeekIncoming() != nil {
		s.pipeline.bottomPort.RetrieveIncoming()
	}

	rsp := &cache.RestartRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = s.ctrlPort.AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficBytes = 0
	rsp.TrafficClass = "ctrl-rsp"

	err := s.ctrlPort.Send(rsp)
	if err != nil {
		log.Panic("Unable to send restart rsp")
	}

	return true
}

func (s *controlStage) shouldWaitForInFlightTransactions() bool {
	next := s.pipeline.comp.GetNextState()
	if s.currFlushReq.DiscardInflight {
		return false
	}
	for i := range next.Transactions {
		if !next.Transactions[i].Removed {
			return true
		}
	}
	return false
}
