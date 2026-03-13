package writeback

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type topParser struct {
	cache *pipelineMW
}

func (p *topParser) Tick() bool {
	if p.cache.state != cacheStateRunning {
		return false
	}

	msg := p.cache.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	next := p.cache.comp.GetNextState()
	if !next.DirStageBuf.CanPush() {
		return false
	}

	trans := &transactionState{
		id: sim.GetIDGenerator().Generate(),
	}

	switch msg := msg.(type) {
	case *mem.ReadReq:
		trans.read = msg
	case *mem.WriteReq:
		trans.write = msg
	}

	p.cache.inFlightTransactions = append(p.cache.inFlightTransactions, trans)

	idx := len(p.cache.inFlightTransactions) - 1
	next.DirStageBuf.PushTyped(idx)

	tracing.TraceReqReceive(msg, p.cache.comp)

	p.cache.topPort.RetrieveIncoming()

	return true
}
