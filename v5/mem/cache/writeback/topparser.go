package writeback

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type topParser struct {
	cache *Comp
}

func (p *topParser) Tick() bool {
	if p.cache.state != cacheStateRunning {
		return false
	}

	msg := p.cache.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	if !p.cache.dirStageBuffer.CanPush() {
		return false
	}

	trans := &transaction{
		id: sim.GetIDGenerator().Generate(),
	}

	switch msg.Payload.(type) {
	case *mem.ReadReqPayload:
		trans.read = msg
	case *mem.WriteReqPayload:
		trans.write = msg
	}

	p.cache.dirStageBuffer.Push(trans)

	p.cache.inFlightTransactions = append(p.cache.inFlightTransactions, trans)

	tracing.TraceReqReceive(msg, p.cache)

	p.cache.topPort.RetrieveIncoming()

	return true
}
