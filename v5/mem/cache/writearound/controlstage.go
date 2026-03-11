package writearound

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

type controlStage struct {
	ctrlPort     sim.Port
	transactions *[]*transaction
	directory    cache.Directory
	cache        *Comp
	coalescer    *coalescer
	bankStages   []*bankStage

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
	s.flushPort(s.cache.topPort)
	s.flushPort(s.cache.bottomPort)
	s.flushBuffer(s.cache.dirBuf)

	for _, bankBuf := range s.cache.bankBufs {
		s.flushBuffer(bankBuf)
	}

	s.directory.Reset()
	s.cache.mshr.Reset()
	s.cache.coalesceStage.Reset()

	for _, bankStage := range s.cache.bankStages {
		bankStage.Reset()
	}

	s.cache.transactions = nil
	s.cache.postCoalesceTransactions = nil

	if s.currFlushReq.PauseAfterFlushing {
		s.cache.isPaused = true
	}
}

func (s *controlStage) flushPort(port sim.Port) {
	for port.PeekIncoming() != nil {
		port.RetrieveIncoming()
	}
}

func (s *controlStage) flushBuffer(buffer queueing.Buffer) {
	for buffer.Pop() != nil {
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
	s.cache.isPaused = false

	s.ctrlPort.RetrieveIncoming()

	for s.cache.topPort.PeekIncoming() != nil {
		s.cache.topPort.RetrieveIncoming()
	}

	for s.cache.bottomPort.PeekIncoming() != nil {
		s.cache.bottomPort.RetrieveIncoming()
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
	return !s.currFlushReq.DiscardInflight && len(s.cache.transactions) != 0
}
