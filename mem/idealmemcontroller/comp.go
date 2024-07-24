package idealmemcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"

	"github.com/sarchlab/akita/v4/sim"

	"github.com/sarchlab/akita/v4/tracing"
)

type readRespondEvent struct {
	*sim.EventBase
	req *mem.ReadReq
}

func newReadRespondEvent(time sim.VTimeInSec, handler sim.Handler,
	req *mem.ReadReq,
) *readRespondEvent {
	return &readRespondEvent{sim.NewEventBase(time, handler), req}
}

type writeRespondEvent struct {
	*sim.EventBase
	req *mem.WriteReq
}

func newWriteRespondEvent(time sim.VTimeInSec, handler sim.Handler,
	req *mem.WriteReq,
) *writeRespondEvent {
	return &writeRespondEvent{sim.NewEventBase(time, handler), req}
}

// An Comp is an ideal memory controller that can perform read and write
// Ideal memory controller always respond to the request in a fixed number of
// cycles. There is no limitation on the concurrency of this unit.
type Comp struct {
	*sim.TickingComponent

	topPort          sim.Port
	CtrlPort         sim.Port
	Storage          *mem.Storage
	Latency          int
	addressConverter mem.AddressConverter

	isDraining       bool
	currentDrainReq  *mem.ControlMsg
	currentPauseReq  *mem.ControlMsg
	currentResetReq  *mem.ControlMsg
	currentEnableReq *mem.ControlMsg

	width int

	enable  bool
	pause   bool
	isPause bool
	drain   bool
	reset   bool
}

// Handle defines how the Comp handles event
func (c *Comp) Handle(e sim.Event) error {
	switch e := e.(type) {
	case *readRespondEvent:
		return c.handleReadRespondEvent(e)
	case *writeRespondEvent:
		return c.handleWriteRespondEvent(e)
	case sim.TickEvent:
		return c.TickingComponent.Handle(e)
	default:
		log.Panicf("cannot handle event of %s", reflect.TypeOf(e))
	}

	return nil
}

func (c *Comp) Tick() bool {
	madeProgress := false

	for i := 0; i < c.width; i++ {
		madeProgress = c.getCtrlSignals() || madeProgress

		if c.enable {
			madeProgress = c.handleCtrlSignals(i) || madeProgress

			if !c.checkStatus() {
				madeProgress = c.handleMemReqs() || madeProgress
			}
		}
	}

	return madeProgress
}

func (c *Comp) checkStatus() bool {
	return c.drain || c.pause
}

func (c *Comp) handleCtrlSignals(i int) bool {
	madeProgress := false

	// if c.reset {
	// 	madeProgress = c.handleReset() || madeProgress
	// } else
	if c.pause {
		madeProgress = c.handlePause() || madeProgress
	} else if c.drain {
		madeProgress = c.handleDrain(i) || madeProgress
	}

	return madeProgress
}

func (c *Comp) getCtrlSignals() bool {
	msg := c.CtrlPort.RetrieveIncoming()

	if msg == nil {
		return false
	}

	tracing.TraceReqReceive(msg, c)

	switch msgType := msg.(type) {
	case *mem.ControlMsg:
		c.handleControlMsg(msgType)
	}

	return true
}

func (c *Comp) handleControlMsg(msg *mem.ControlMsg) {
	if c.enable != msg.Enable {
		c.currentEnableReq = msg
		c.enable = msg.Enable
	}

	if c.pause != msg.Pause {
		c.currentPauseReq = msg
		c.pause = msg.Pause
	}

	if c.drain != msg.Drain {
		c.currentDrainReq = msg
		c.drain = msg.Drain

		if c.drain {
			c.isDraining = true
		}
	}

	// if c.reset != msg.Reset {
	// 	c.currentResetReq = msg
	// 	c.reset = msg.Reset
	// }

	if msg.Reset {
		c.currentResetReq = msg
		c.reset = true

		c.handleReset()
	}
}

func (c *Comp) handleReset() bool {
	resetCompleteRsp := sim.GeneralRspBuilder{}.
		WithSrc(c.CtrlPort).
		WithDst(c.currentResetReq.Src).
		WithOriginalReq(c.currentResetReq).
		Build()
	err := c.CtrlPort.Send(resetCompleteRsp)

	if err != nil {
		return false
	}

	c.drain = false
	c.isDraining = false

	c.pause = false
	c.isPause = false

	c.reset = false

	return true
}

func (c *Comp) handlePause() bool {
	if !c.isPause {
		responsePauseReq := sim.GeneralRspBuilder{}.
			WithSrc(c.CtrlPort).
			WithDst(c.currentPauseReq.Src).
			WithOriginalReq(c.currentPauseReq).
			Build()

		err := c.CtrlPort.Send(responsePauseReq)

		if err != nil {
			return false
		}

		c.isPause = true
	}

	return true
}

func (c *Comp) handleDrain(i int) bool {
	madeProgress := false

	if c.fullyDrained(i) {
		drainCompleteRsp := sim.GeneralRspBuilder{}.
			WithSrc(c.CtrlPort).
			WithDst(c.currentDrainReq.Src).
			WithOriginalReq(c.currentDrainReq).
			Build()

		err := c.CtrlPort.Send(drainCompleteRsp)

		if err != nil {
			return false
		}
		c.isDraining = false
		return true
	}

	madeProgress = c.handleMemReqs()

	return madeProgress
}

func (c *Comp) fullyDrained(i int) bool {
	if (i == c.width-1) || c.topPort.PeekIncoming() == nil {
		return true
	}

	return false
}

// updateMemCtrl updates ideal memory controller state.
func (c *Comp) handleMemReqs() bool {
	msg := c.topPort.RetrieveIncoming()

	if msg == nil {
		return false
	}

	tracing.TraceReqReceive(msg, c)

	switch msg := msg.(type) {
	case *mem.ReadReq:
		c.handleReadReq(msg)
		return true
	case *mem.WriteReq:
		c.handleWriteReq(msg)
		return true
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(msg))
	}

	return false
}

func (c *Comp) handleReadReq(req *mem.ReadReq) {
	now := c.CurrentTime()
	timeToSchedule := c.Freq.NCyclesLater(c.Latency, now)
	respondEvent := newReadRespondEvent(timeToSchedule, c, req)
	c.Engine.Schedule(respondEvent)
}

func (c *Comp) handleWriteReq(req *mem.WriteReq) {
	now := c.CurrentTime()
	timeToSchedule := c.Freq.NCyclesLater(c.Latency, now)
	respondEvent := newWriteRespondEvent(timeToSchedule, c, req)
	c.Engine.Schedule(respondEvent)
}

func (c *Comp) handleReadRespondEvent(e *readRespondEvent) error {
	now := e.Time()
	req := e.req

	addr := req.Address

	if c.addressConverter != nil {
		addr = c.addressConverter.ConvertExternalToInternal(addr)
	}

	data, err := c.Storage.Read(addr, req.AccessByteSize)

	if err != nil {
		log.Panic(err)
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(data).
		Build()

	networkErr := c.topPort.Send(rsp)

	if networkErr != nil {
		retry := newReadRespondEvent(c.Freq.NextTick(now), c, req)
		c.Engine.Schedule(retry)
		return nil
	}

	tracing.TraceReqComplete(req, c)
	c.TickLater()

	return nil
}

func (c *Comp) handleWriteRespondEvent(e *writeRespondEvent) error {
	now := e.Time()
	req := e.req

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()

	networkErr := c.topPort.Send(rsp)

	if networkErr != nil {
		retry := newWriteRespondEvent(c.Freq.NextTick(now), c, req)
		c.Engine.Schedule(retry)
		return nil
	}

	addr := req.Address

	if c.addressConverter != nil {
		addr = c.addressConverter.ConvertExternalToInternal(addr)
	}

	if req.DirtyMask == nil {
		err := c.Storage.Write(addr, req.Data)

		if err != nil {
			log.Panic(err)
		}
	} else {
		data, err := c.Storage.Read(addr, uint64(len(req.Data)))

		if err != nil {
			panic(err)
		}
		for i := 0; i < len(req.Data); i++ {
			if req.DirtyMask[i] {
				data[i] = req.Data[i]
			}
		}
		err = c.Storage.Write(addr, data)

		if err != nil {
			panic(err)
		}
	}

	tracing.TraceReqComplete(req, c)
	c.TickLater()

	return nil
}

func (c *Comp) CurrentTime() sim.VTimeInSec {
	return c.Engine.CurrentTime()
}
