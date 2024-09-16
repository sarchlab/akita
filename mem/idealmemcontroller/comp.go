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
	sim.MiddlewareHolder

	topPort          sim.Port
	CtrlPort         sim.Port
	Storage          *mem.Storage
	Latency          int
	addressConverter mem.AddressConverter

	respondReq *mem.ControlMsg
	width      int

	state     string
	ctrlState mem.Pause

	inflightbuffer []sim.Msg
}

func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.handleCtrlSignals() || madeProgress
	madeProgress = c.updateInflightBuffer() || madeProgress
	madeProgress = c.handleBehavior() || madeProgress

	return madeProgress
}

func (c *Comp) handleBehavior() bool {
	madeProgress := false

	switch state := c.state; state {
	case "enable":
		madeProgress = c.handleInflightMemReqs()
	case "pause":
		madeProgress = true
	case "drain":
		madeProgress = c.handleDrainReq()
	}

	return madeProgress
}

func (c *Comp) handleDrainReq() bool {
	madeProgress := false
	for len(c.inflightbuffer) != 0 {
		madeProgress = c.handleMemReqs()
	}
	if !c.setState("pause", c.respondReq) {
		return true
	}
	return madeProgress
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

func (c *Comp) handleCtrlSignals() (madeProgress bool) {
	for {
		msg := c.CtrlPort.PeekIncoming()
		if msg == nil {
			return madeProgress
		}

		if c.state == "drain" {
			return madeProgress
		}

		signalMsg := msg.(*mem.ControlMsg)

		if c.handleInvalidOrFlushSignal(signalMsg) {
			return madeProgress
		}

		if c.handleEnableSignal(signalMsg) {
			return true
		}

		if c.handleDrainSignal(signalMsg) {
			return true
		}

		if c.handlePauseSignal(signalMsg) {
			return true
		}

		c.CtrlPort.RetrieveIncoming()
	}
}

func (c *Comp) handleInvalidOrFlushSignal(signalMsg *mem.ControlMsg) bool {
	if signalMsg.Pause.Invalid || signalMsg.Pause.Flush {
		panic("Invalid or Flush signal should not be sent to ideal memory controller")
	}
	return false
}

func (c *Comp) handleEnableSignal(signalMsg *mem.ControlMsg) bool {
	if signalMsg.Pause.Enable && !signalMsg.Pause.Drain {
		madeProgress := c.setState("enable", signalMsg)
		return madeProgress
	}
	return false
}

func (c *Comp) handleDrainSignal(signalMsg *mem.ControlMsg) bool {
	if !signalMsg.Pause.Enable && signalMsg.Pause.Drain {
		c.state = "drain"
		c.respondReq = signalMsg
		return true
	}
	return false
}

func (c *Comp) handlePauseSignal(signalMsg *mem.ControlMsg) bool {
	if !signalMsg.Pause.Enable && !signalMsg.Pause.Drain {
		madeProgress := c.setState("pause", signalMsg)
		return madeProgress
	}
	return false
}

func (c *Comp) updateInflightBuffer() bool {
	if c.state == "pause" {
		return false
	}

	for i := 0; i < c.width; i++ {
		msg := c.topPort.RetrieveIncoming()
		if msg == nil {
			return false
		}

		c.inflightbuffer = append(c.inflightbuffer, msg)
	}
	return true
}

// updateMemCtrl updates ideal memory controller state.
func (c *Comp) handleInflightMemReqs() bool {
	madeProgress := false
	for i := 0; i < c.width; i++ {
		madeProgress = c.handleMemReqs()
	}

	return madeProgress
}

func (c *Comp) handleMemReqs() bool {
	if len(c.inflightbuffer) == 0 {
		return false
	}

	msg := c.inflightbuffer[0]
	c.inflightbuffer = c.inflightbuffer[1:]

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

func (c *Comp) setState(state string, rspMessage *mem.ControlMsg) bool {
	ctrlRsp := sim.GeneralRspBuilder{}.
		WithSrc(c.CtrlPort).
		WithDst(rspMessage.Src).
		WithOriginalReq(rspMessage).
		Build()

	err := c.CtrlPort.Send(ctrlRsp)

	if err != nil {
		return false
	}

	c.CtrlPort.RetrieveIncoming()
	c.state = state
	c.ctrlState = rspMessage.Pause

	return true
}
