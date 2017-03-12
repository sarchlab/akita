package main

import (
	"errors"
	"fmt"

	"gitlab.com/yaotsu/core"

	"reflect"
)

// A PingComponent periodically send ping request out and also respond to pings
//
// -----------------
// |               |
// | PingComponent | <=> Ping
// |               |
// -----------------
//
type PingComponent struct {
	*core.BasicComponent

	NumPingsToSend int
	Engine         core.Engine
}

// NewPingComponent creates a new PingComponent
func NewPingComponent(name string, engine core.Engine) *PingComponent {
	c := &PingComponent{
		core.NewBasicComponent(name),
		0,
		engine,
	}
	c.AddPort("Ping")
	return c
}

// Receive processes incoming request
func (c *PingComponent) Receive(req core.Request) *core.Error {
	switch req := req.(type) {
	default:
		return core.NewError(
			"cannot process request "+reflect.TypeOf(req).String(),
			false, 0)
	case *PingReq:
		return c.processPingReq(req)
	}
}

func (c *PingComponent) processPingReq(req *PingReq) *core.Error {
	if req.IsReply {
		fmt.Printf("Component %s: ping time=%f s\n", c.Name(),
			req.RecvTime()-req.StartTime)
		return nil
	}

	evt := NewPingReturnEvent()
	evt.Req = req
	evt.SetTime(req.RecvTime() + 2.0)
	evt.SetHandler(c)
	c.Engine.Schedule(evt)
	return nil
}

// Handle handles the event for the PingComponent
func (c *PingComponent) Handle(e core.Event) error {
	switch e := e.(type) {
	default:
		return errors.New("cannot handle event " + reflect.TypeOf(e).String())
	case *PingReturnEvent:
		return c.handlePingReturnEvent(e)
	case *PingSendEvent:
		return c.handlePingSendEvent(e)
	}
}

func (c *PingComponent) handlePingReturnEvent(e *PingReturnEvent) error {
	e.Req.SwapSrcAndDst()
	e.Req.IsReply = true

	// Send the reply
	e.Req.SetSendTime(e.Time())
	err := c.GetConnection("Ping").Send(e.Req)
	if err != nil {
		if !err.Recoverable {
			return err
		}

		// Reschedule
		e.Req.SwapSrcAndDst()
		e.SetTime(err.EarliestRetry)
		c.Engine.Schedule(e)
	}

	e.FinishChan() <- true
	return nil
}

func (c *PingComponent) handlePingSendEvent(e *PingSendEvent) error {
	if e.From != c {
		panic("Ping event is not scheduled for the current component")
	}

	req := NewPingReq()
	req.SetSource(e.From)
	req.SetDestination(e.To)
	req.StartTime = e.Time()
	req.SetSendTime(e.Time())

	err := c.GetConnection("Ping").Send(req)
	if err != nil {
		if !err.Recoverable {
			return err
		}

		e.SetTime(err.EarliestRetry)
		c.Engine.Schedule(e)
	}

	e.FinishChan() <- true
	return nil
}
