package writeback

import (
	"log"

	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

type ctrlMiddleware struct {
	*Comp

	flushReq *cache.FlushReq
}

func (c *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = c.handleIncomingCommands() || madeProgress
	return madeProgress
}

func (c *ctrlMiddleware) handleIncomingCommands() bool {
	madeProgress := false

	msg := c.controlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	if c.cacheStateMustBeValid() {
		madeProgress = c.ctrlMsgMustBeValid(msg.(*mem.ControlReq)) || madeProgress
		madeProgress = c.perfCtrlReq() || madeProgress
	}

	return madeProgress
}

func (c *ctrlMiddleware) cacheStateMustBeValid() bool {
	if c.state == "enable" || c.state == "pause" {
		return true
	}

	return false
}

func (c *ctrlMiddleware) ctrlMsgMustBeValid(msg sim.Msg) bool {
	madeProgress := true

	// if msg.Enable {
	// 	if c.state == "pause" {
	// 		c.state = "enable"
	// 		madeProgress = true
	// 	} else if c.state == "enable" {
	// 		// Already enabled, no action needed.
	// 	} else {
	// 		panic("Invalid state transition")
	// 	}
	// } else if msg.Pause {
	// 	if c.state == "enable" {
	// 		c.state = "pause"
	// 		madeProgress = true
	// 	} else if c.state == "pause" {
	// 		// Already paused, no action needed.
	// 	} else {
	// 		panic("Invalid state transition")
	// 	}
	// } else if msg.Drain {
	// 	c.state = "drain"
	// 	madeProgress = true
	// } else {
	// 	panic("Unhandled control message")

	// }
	return madeProgress
}

func (c *ctrlMiddleware) perfCtrlReq() bool {
	madeProgress := false

	msg := c.controlPort.PeekIncoming()
	switch req := msg.(type) {
	case *cache.FlushReq:
		madeProgress = c.handleFlushReq(req) || madeProgress
	case *cache.RestartReq:
		madeProgress = c.handleRestartReq(req) || madeProgress
	case *mem.ControlReq:
		madeProgress = c.handleControlReq(req) || madeProgress
	default:
		panic("Unhandled control message")
	}

	return madeProgress
}

func (c *ctrlMiddleware) handleFlushReq(req *cache.FlushReq) bool {
	madeProgress := false

	madeProgress = c.saveFlushReq(req) || madeProgress

	if c.flushReq != nil && c.isDrained {
		madeProgress = c.processFlushQueue() || madeProgress
	}
	return true
}

func (c *ctrlMiddleware) saveFlushReq(req *cache.FlushReq) bool {
	c.flushReq = req
	return true
}

func (c *ctrlMiddleware) processFlushQueue() bool {
	if c.flushReq == nil {
		return false
	}

	rsp := cache.FlushRspBuilder{}.
		WithSrc(c.controlPort.AsRemote()).
		WithDst(c.flushReq.Src).
		WithRspTo(c.flushReq.ID).
		Build()

	err := c.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	c.hardResetCache()
	c.flushReq = nil

	return true
}

func (c *ctrlMiddleware) hardResetCache() {
	c.flushPort(c.topPort)
	c.flushPort(c.bottomPort)
	c.flushBuffer(c.dirBuf)

	for _, bankBuf := range c.bankBufs {
		c.flushBuffer(bankBuf)
	}

	c.directory.Reset()
	c.mshr.Reset()
	c.coalesceStage.Reset()

	for _, bankStage := range c.bankStages {
		bankStage.Reset()
	}

	c.transactions = nil
	c.postCoalesceTransactions = nil

	if c.flushReq.PauseAfterFlushing {
		c.isPaused = true
	}
}

func (c *ctrlMiddleware) flushPort(port sim.Port) {
	for port.PeekIncoming() != nil {
		port.RetrieveIncoming()
	}
}

func (c *ctrlMiddleware) flushBuffer(buffer sim.Buffer) {
	for buffer.Pop() != nil {
	}
}

func (c *ctrlMiddleware) handleRestartReq(req *cache.RestartReq) bool {
	c.isPaused = false

	for c.topPort.PeekIncoming() != nil {
		c.topPort.RetrieveIncoming()
	}

	for c.bottomPort.PeekIncoming() != nil {
		c.bottomPort.RetrieveIncoming()
	}

	rsp := cache.RestartRspBuilder{}.
		WithSrc(c.controlPort.AsRemote()).
		WithDst(req.Src).
		Build()

	c.controlPort.RetrieveIncoming()

	err := c.controlPort.Send(rsp)
	if err != nil {
		log.Panic("Unable to send restart rsp")
	}

	c.state = "enable"

	return true
}

func (c *ctrlMiddleware) handleControlReq(req *mem.ControlReq) bool {
	madeProgress := false

	if req.Enable {
		c.state = "enable"
		madeProgress = true
		c.controlPort.RetrieveIncoming()
	} else if req.Drain {
		c.state = "drain"
		madeProgress = true
	} else if req.Pause {
		c.state = "pause"
		madeProgress = true
		c.controlPort.RetrieveIncoming()
	}

	rsp := mem.ControlRspBuilder{}.
		WithSrc(c.controlPort.AsRemote()).
		WithDst(req.Src).
		WithCtrlInfo(
			c.state == "enable",
			c.state == "drain",
			c.state == "pause").
		Build()

	err := c.controlPort.Send(rsp)
	if err != nil {
		log.Panic("Unable to send control rsp")
	}

	return madeProgress
}
