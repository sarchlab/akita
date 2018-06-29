package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	_ "net/http/pprof"
	"reflect"

	"gitlab.com/yaotsu/core"
)

var cpuprofile = flag.String("cpuprofile", "cpuprof.prof", "write cpu profile to file")

func main() {
	// flag.Parse()
	// if *cpuprofile != "" {
	// 	f, err := os.Create(*cpuprofile)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	pprof.StartCPUProfile(f)
	// 	defer pprof.StopCPUProfile()
	// }

	// runtime.SetBlockProfileRate(1)
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	engine := core.NewSerialEngine()
	connection := core.NewDirectConnection(engine)

	numAgents := 4

	agents := make([]*PingComponent, 0)
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("agent%d", i)
		agent := NewPingComponent(name, engine)
		connection.PlugIn(agent, "Ping")
		agents = append(agents, agent)
	}

	for i := 0; i < 1000; i++ {

		from := rand.Uint32() % uint32(numAgents)
		to := rand.Uint32() % uint32(numAgents)
		time := rand.Uint32() % 100

		evt := NewPingSendEvent(core.VTimeInSec(time), agents[from])

		evt.From = agents[from]
		evt.To = agents[to]

		engine.Schedule(evt)
	}

	engine.Run()
}

// A PingComponent periodically send ping request out and also respond to pings
//
//     -----------------
//     |               |
//     | PingComponent | <=> Ping
//     |               |
//     -----------------
//
type PingComponent struct {
	*core.ComponentBase

	NumPingsToSend int
	Engine         core.Engine
}

// NewPingComponent creates a new PingComponent
func NewPingComponent(name string, engine core.Engine) *PingComponent {
	c := &PingComponent{
		core.NewComponentBase(name),
		0,
		engine,
	}
	c.AddPort("Ping")
	return c
}

// Recv processes incoming request
func (c *PingComponent) Recv(req core.Req) *core.SendError {
	switch req := req.(type) {
	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(req))
	case *PingReq:
		return c.processPingReq(req)
	}
	return nil
}

func (c *PingComponent) processPingReq(req *PingReq) *core.SendError {
	if req.IsReply {
		fmt.Printf("Component %s: ping time=%f s\n", c.Name(),
			req.RecvTime()-req.StartTime)
		return nil
	}

	evt := NewPingReturnEvent(req.RecvTime()+2.0, c)
	evt.Req = req
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
		// Reschedule
		e.Req.SwapSrcAndDst()
		e.SetTime(e.Time() + 0.01)
		c.Engine.Schedule(e)
	}

	return nil
}

func (c *PingComponent) handlePingSendEvent(e *PingSendEvent) error {
	if e.From != c {
		panic("Ping event is not scheduled for the current component")
	}

	req := NewPingReq()
	req.SetSrc(e.From)
	req.SetDst(e.To)
	req.StartTime = e.Time()
	req.SetSendTime(e.Time())

	err := c.GetConnection("Ping").Send(req)
	if err != nil {
		e.SetTime(e.Time() + 0.01)
		c.Engine.Schedule(e)
	}

	return nil
}

// A PingReq is the Ping message send from one node to another
type PingReq struct {
	*core.ReqBase

	StartTime core.VTimeInSec
	IsReply   bool
}

// NewPingReq creates a new PingReq
func NewPingReq() *PingReq {
	return &PingReq{core.NewReqBase(), 0, false}
}

// A PingReturnEvent is an event scheduled for returning the ping request
type PingReturnEvent struct {
	*core.EventBase
	Req *PingReq
}

// NewPingReturnEvent creates a new PingReturnEvent
func NewPingReturnEvent(
	t core.VTimeInSec,
	handler core.Handler,
) *PingReturnEvent {
	return &PingReturnEvent{core.NewEventBase(t, handler), nil}
}

// A PingSendEvent is an event scheduled for sending a ping
type PingSendEvent struct {
	*core.EventBase
	From *PingComponent
	To   *PingComponent
}

// NewPingSendEvent creates a new PingSendEvent
func NewPingSendEvent(
	time core.VTimeInSec,
	handler core.Handler,
) *PingSendEvent {
	return &PingSendEvent{core.NewEventBase(time, handler), nil, nil}
}
