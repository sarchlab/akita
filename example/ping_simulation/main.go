package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	_ "net/http/pprof"
	"reflect"

	"gitlab.com/akita/akita"
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

	//engine := akita.NewSerialEngine()
	engine := akita.NewParallelEngine()
	connection := akita.NewDirectConnection(engine)

	numAgents := 4

	agents := make([]*PingComponent, 0)
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("agent%d", i)
		agent := NewPingComponent(name, engine)
		connection.PlugIn(agent.ToOut)
		agents = append(agents, agent)
	}

	for i := 0; i < 10000; i++ {

		from := rand.Uint32() % uint32(numAgents)
		to := rand.Uint32() % uint32(numAgents)
		time := rand.Float64() / 1e8

		evt := NewPingSendEvent(akita.VTimeInSec(time), agents[from])

		evt.From = agents[from]
		evt.To = agents[to]

		engine.Schedule(evt)
	}

	engine.Run()
}

// A PingComponent periodically send ping request out and also respond to pings
type PingComponent struct {
	*akita.ComponentBase

	NumPingsToSend int
	Engine         akita.Engine
	ToOut          akita.Port
	Freq           akita.Freq
}

func (c *PingComponent) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
	panic("implement me")
}

func (c *PingComponent) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, c.Engine, c.Freq)
}

// NewPingComponent creates a new PingComponent
func NewPingComponent(name string, engine akita.Engine) *PingComponent {
	c := new(PingComponent)
	c.ComponentBase = akita.NewComponentBase(name)
	c.Engine = engine
	c.Freq = 1 * akita.GHz

	c.ToOut = akita.NewPort(c)
	return c
}

// Handle handles the event for the PingComponent
func (c *PingComponent) Handle(e akita.Event) error {
	switch e := e.(type) {
	default:
		return errors.New("cannot handle event " + reflect.TypeOf(e).String())
	case *PingReq:
		return c.processPingReq(e)
	case *PingReturnEvent:
		return c.handlePingReturnEvent(e)
	case *PingSendEvent:
		return c.handlePingSendEvent(e)
	}
}

func (c *PingComponent) processPingReq(req *PingReq) error {
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

func (c *PingComponent) handlePingReturnEvent(e *PingReturnEvent) error {
	now := e.Time()
	e.Req.SwapSrcAndDst()
	e.Req.IsReply = true

	// Send the reply
	e.Req.SetSendTime(e.Time())
	err := c.ToOut.Send(e.Req)
	if err != nil {
		// Reschedule
		e.Req.SwapSrcAndDst()
		newEvent := NewPingReturnEvent(c.Freq.NextTick(now), c)
		c.Engine.Schedule(newEvent)
	}

	return nil
}

func (c *PingComponent) handlePingSendEvent(e *PingSendEvent) error {
	now := e.Time()

	req := NewPingReq()
	req.SetSrc(e.From.ToOut)
	req.SetDst(e.To.ToOut)
	req.StartTime = e.Time()
	req.SetSendTime(e.Time())

	err := c.ToOut.Send(req)
	if err != nil {
		newEvt := NewPingSendEvent(c.Freq.NextTick(now), c)
		c.Engine.Schedule(newEvt)
	}

	return nil
}

// A PingReq is the Ping message send from one node to another
type PingReq struct {
	*akita.ReqBase

	StartTime akita.VTimeInSec
	IsReply   bool
}

// NewPingReq creates a new PingReq
func NewPingReq() *PingReq {
	return &PingReq{akita.NewReqBase(), 0, false}
}

// A PingReturnEvent is an event scheduled for returning the ping request
type PingReturnEvent struct {
	*akita.EventBase
	Req *PingReq
}

// NewPingReturnEvent creates a new PingReturnEvent
func NewPingReturnEvent(
	t akita.VTimeInSec,
	handler akita.Handler,
) *PingReturnEvent {
	return &PingReturnEvent{akita.NewEventBase(t, handler), nil}
}

// A PingSendEvent is an event scheduled for sending a ping
type PingSendEvent struct {
	*akita.EventBase
	From *PingComponent
	To   *PingComponent
}

// NewPingSendEvent creates a new PingSendEvent
func NewPingSendEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
) *PingSendEvent {
	return &PingSendEvent{akita.NewEventBase(time, handler), nil, nil}
}
