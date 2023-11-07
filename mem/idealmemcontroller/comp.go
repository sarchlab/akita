package idealmemcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/pipelining"
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

type msgItem struct {
	ms *sim.Msg
}

func (m msgItem) TaskID() string {
	return (*m.ms).Meta().ID
}

// An Comp is an ideal memory controller that can perform read and write
// Ideal memory controller always respond to the request in a fixed number of
// cycles. There is no limitation on the concurrency of this unit.
type Comp struct {
	*sim.TickingComponent

	topPort            sim.Port
	Storage            *mem.Storage
	Latency            int
	AddressConverter   mem.AddressConverter
	MaxNumTransaction  int
	currNumTransaction int

	pipeline         pipelining.Pipeline
	postPipelineBuf  sim.Buffer
	numStage         int
	numCyclePerStage int
	width            int
}

func (c *Comp) CanAcceptMsg() bool {
	return c.pipeline.CanAccept()
}

// func (c *Comp) AcceptMsg(msg *sim.Msg, now sim.VTimeInSec) {
// 	c.pipeline.Accept(now, msgItem{msg: msg.(*)})
// }

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

	if c.currNumTransaction >= c.MaxNumTransaction {
		return false
	}

	for i := 0; i < c.width; i++ {
		madeProgress = c.msgFromPortToPipeline(now) || madeProgress
	}
	madeProgress = c.pipeline.Tick(now) || madeProgress

	for i := 0; i < c.width; i++ {
		madeProgress = c.upDateMemCtrl(now) || madeProgress
	}

	return madeProgress
}

func (c *Comp) msgFromPortToPipeline(now sim.VTimeInSec) bool {
	msg := c.topPort.Retrieve(now)

	if msg == nil {
		return false
	}

	if !c.pipeline.CanAccept() {
		return false
	}

	c.pipeline.Accept(now, msgItem{ms: &msg})
	return false
}

// Tick updates ideal memory controller state.
func (c *Comp) upDateMemCtrl(now sim.VTimeInSec) bool {
	// if c.currNumTransaction >= c.MaxNumTransaction {
	// 	return false
	// }

	// for i := 0; i < c.width; i++ {
	// 	msg := c.topPort.Retrieve(now)
	// 	if msg == nil {
	// 		return false
	// 	}
	item := c.postPipelineBuf.Peek()
	if item == nil {
		return false
	}

	req := item.(msgItem).ms
	tracing.TraceReqReceive(*req, c)
	c.currNumTransaction++

	switch msg := (*req).(type) {
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
	c.postPipelineBuf.Pop()
}

func (c *Comp) handleWriteReq(now sim.VTimeInSec, req *mem.WriteReq) {
	timeToSchedule := c.Freq.NCyclesLater(c.Latency, now)
	respondEvent := newWriteRespondEvent(timeToSchedule, c, req)
	c.Engine.Schedule(respondEvent)
	c.postPipelineBuf.Pop()
}

func (c *Comp) handleReadRespondEvent(e *readRespondEvent) error {
	now := e.Time()
	req := e.req

	addr := req.Address
	if c.AddressConverter != nil {
		addr = c.AddressConverter.ConvertExternalToInternal(addr)
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
	// c.postPipelineBuf.Pop()
	c.currNumTransaction--
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

	if c.AddressConverter != nil {
		addr = c.AddressConverter.ConvertExternalToInternal(addr)
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
	// c.postPipelineBuf.Pop()
	c.currNumTransaction--
	c.TickLater(now)

	return nil
}

func (c *Comp) Reset() {
	c.pipeline.Clear()
	c.postPipelineBuf.Clear()
}
