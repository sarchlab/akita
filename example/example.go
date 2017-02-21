package example

import (
	"errors"
	"fmt"

	"reflect"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

// A PingReq is the Ping message send from one node to another
type PingReq struct {
	*conn.BasicRequest

	StartTime event.VTimeInSec
	IsReply   bool
}

// NewPingReq creates a new PingReq
func NewPingReq() *PingReq {
	return &PingReq{conn.NewBasicRequest(), 0, false}
}

// A PingSendEvent is an event scheduled for sending a ping
type PingSendEvent struct {
	*event.BasicEvent
	From *PingComponent
	To   *PingComponent
}

// NewPingSendEvent creates a new PingSendEvent
func NewPingSendEvent() *PingSendEvent {
	return &PingSendEvent{event.NewBasicEvent(), nil, nil}
}

// A PingReturnEvent is an event scheduled for returning the ping request
type PingReturnEvent struct {
	*event.BasicEvent
	Req *PingReq
}

// NewPingReturnEvent creates a new PingReturnEvent
func NewPingReturnEvent() *PingReturnEvent {
	return &PingReturnEvent{event.NewBasicEvent(), nil}
}

// A PingComponent periodically send ping request out and also respond to pings
//
// -----------------
// |               |
// | PingComponent | <=> Ping
// |               |
// -----------------
//
type PingComponent struct {
	*conn.BasicComponent

	NumPingsToSend int
	Engine         event.Engine

	reqChan chan conn.Request
}

// NewPingComponent creates a new PingComponent
func NewPingComponent(name string, engine event.Engine) *PingComponent {
	c := &PingComponent{
		conn.NewBasicComponent(name),
		0,
		engine,
		make(chan conn.Request),
	}
	c.AddPort("Ping")
	return c
}

// Receive processes incoming request
func (c *PingComponent) Receive(req conn.Request) *conn.Error {
	switch req := req.(type) {
	default:
		return conn.NewError(
			"cannot process request "+reflect.TypeOf(req).String(),
			false, 0)
	case *PingReq:
		return c.processPingReq(req)
	}
}

func (c *PingComponent) processPingReq(req *PingReq) *conn.Error {
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
func (c *PingComponent) Handle(e event.Event) error {
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
