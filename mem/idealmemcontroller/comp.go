package idealmemcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"

	"github.com/sarchlab/akita/v3/tracing"
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

	isDraining        bool
	pauseIncomingReqs bool
	currentDrainRsp   *mem.ControlMsg
	currentPauseRsp   *mem.ControlMsg
	currentResetRsp   *mem.ControlMsg
	currentEnableRsp  *mem.ControlMsg

	width int

	enable   bool
	isEnable bool
	pause    bool
	isPause  bool
	drain    bool
	reset    bool
	isReset  bool
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

func (c *Comp) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < c.width; i++ {
		madeProgress = c.processControlSignals(now) || madeProgress
		if c.enable {
			if c.reset {
				madeProgress = c.updateCtrls(now, i) || madeProgress
			} else {
				madeProgress = c.updateMemCtrl(now) || madeProgress
			}
		}
	}
	return madeProgress
}

func (c *Comp) updateCtrls(now sim.VTimeInSec, i int) bool {
	madeProgress := false
	if c.reset {
		madeProgress = c.handleReset(now) || madeProgress
	} else if c.pause {
		madeProgress = c.handlePauseProcess(now) || madeProgress
	} else if c.drain {
		madeProgress = c.handleDrain(now, i) || madeProgress
	}
	return madeProgress
}

func (c *Comp) processControlSignals(now sim.VTimeInSec) bool {
	msg := c.CtrlPort.Retrieve(now)
	if msg == nil {
		return false
	}

	tracing.TraceReqReceive(msg, c)

	switch msgType := msg.(type) {
	case *mem.ControlMsg:
		c.handelControlMsg(msgType)
	}

	return true
}

func (c *Comp) handelControlMsg(msg *mem.ControlMsg) {
	if c.enable != msg.Enable {
		c.currentEnableRsp = msg
		c.enable = msg.Enable
	}

	if c.pause != msg.Pause {
		c.currentPauseRsp = msg
		c.pause = msg.Pause
	}

	if c.drain != msg.Drain {
		c.currentDrainRsp = msg
		c.drain = msg.Drain

		if c.drain {
			c.pauseIncomingReqs = true
			c.isDraining = true
		}
	}

	if c.reset != msg.Reset {
		c.currentResetRsp = msg
		c.reset = msg.Reset
	}
}

func (c *Comp) handleReset(now sim.VTimeInSec) bool {
	resetCompleteRsp := mem.ResetRspBuilder{}.
		WithSendTime(now).
		WithSrc(c.CtrlPort).
		WithDst(c.currentResetRsp.Src).
		Build()
	err := c.CtrlPort.Send(resetCompleteRsp)

	if err != nil {
		return false
	}

	c.isDraining = false
	c.pauseIncomingReqs = false
	c.enable = true
	c.isEnable = true
	c.pause = false
	c.isPause = false
	c.drain = false
	c.reset = false
	c.isReset = false

	return true
}

func (c *Comp) handlePauseProcess(now sim.VTimeInSec) bool {
	if !c.isPause {
		responsePauseReq := mem.PauseRspBuilder{}.
			WithSendTime(now).
			WithSrc(c.CtrlPort).
			WithDst(c.currentPauseRsp.Src).
			Build()

		err := c.CtrlPort.Send(responsePauseReq)

		if err != nil {
			return false
		}

		c.isPause = true
	}

	return true
}

func (c *Comp) handleDrain(now sim.VTimeInSec, i int) bool {
	madeProgress := false
	if c.fullyDrained(i) {
		drainCompleteRsp := mem.DrainRspBuilder{}.
			WithSendTime(now).
			WithSrc(c.CtrlPort).
			WithDst(c.currentDrainRsp.Src).
			Build()

		err := c.CtrlPort.Send(drainCompleteRsp)
		if err != nil {
			return false
		}
		c.isDraining = false
		return true
	}

	madeProgress = c.updateMemCtrl(now)
	return madeProgress
}

func (c *Comp) fullyDrained(i int) bool {
	if (i == c.width-1) || c.topPort.Peek() == nil {
		return true
	}
	return false
}

// updateMemCtrl updates ideal memory controller state.
func (c *Comp) updateMemCtrl(now sim.VTimeInSec) bool {
	msg := c.topPort.Retrieve(now)
	if msg == nil {
		return false
	}

	tracing.TraceReqReceive(msg, c)

	switch msg := msg.(type) {
	case *mem.ReadReq:
		c.handleReadReq(now, msg)
		return true
	case *mem.WriteReq:
		c.handleWriteReq(now, msg)
		return true
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(msg))
	}
	return false
}

func (c *Comp) handleReadReq(now sim.VTimeInSec, req *mem.ReadReq) {
	timeToSchedule := c.Freq.NCyclesLater(c.Latency, now)
	respondEvent := newReadRespondEvent(timeToSchedule, c, req)
	c.Engine.Schedule(respondEvent)
}

func (c *Comp) handleWriteReq(now sim.VTimeInSec, req *mem.WriteReq) {
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
		WithSendTime(now).
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
	c.TickLater(now)

	return nil
}

func (c *Comp) handleWriteRespondEvent(e *writeRespondEvent) error {
	now := e.Time()
	req := e.req

	rsp := mem.WriteDoneRspBuilder{}.
		WithSendTime(now).
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
	c.TickLater(now)

	return nil
}
