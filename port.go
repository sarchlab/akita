package akita

import (
	"log"
	"sync"
)

// A Port is owned by a component and is used to plugin connections
type Port interface {
	SetConnection(conn Connection)
	Component() Component

	// For connection
	Recv(req Req) *SendError
	NotifyAvailable(now VTimeInSec)

	// For component
	Send(req Req) *SendError
	Retrieve(now VTimeInSec) Req
	Peek() Req
}

// PortEndSimulationChecker checks if the port buffer is empty at the end of
// the simulation. If the port is not empty, there is something wrong in the
// simulation.
type PortEndSimulationChecker struct {
	Port Port
}

// Handle checks if the port is empty or not.
func (c *PortEndSimulationChecker) Handle(e Event) error {
	if c.Port.Peek() != nil {
		log.Panic("port is not free")
	}
	return nil
}

// LimitNumReqPort is a type of port that can hold at most a certain number
// of requests.
type LimitNumReqPort struct {
	sync.Mutex

	Buf         []Req
	BufCapacity int
	PortBusy    bool

	Conn     Connection
	ConnBusy bool

	Comp Component
}

// SetConnection sets which connection plugged in to this port.
func (p *LimitNumReqPort) SetConnection(conn Connection) {
	p.Conn = conn
}

// Component returns the owner component of the port.
func (p *LimitNumReqPort) Component() Component {
	return p.Comp
}

// Send is used to send a request out from a component
func (p *LimitNumReqPort) Send(req Req) *SendError {
	err := p.Conn.Send(req)
	if err != nil {
		p.Lock()
		p.ConnBusy = true
		p.Unlock()
	}
	return err
}

// Recv is used to deliver a request to a component
func (p *LimitNumReqPort) Recv(req Req) *SendError {
	p.Lock()
	if len(p.Buf) >= p.BufCapacity {
		p.PortBusy = true
		p.Unlock()
		return NewSendError()
	}

	p.Buf = append(p.Buf, req)
	p.Unlock()

	if p.Comp != nil {
		p.Comp.NotifyRecv(req.RecvTime(), p)
	}
	return nil
}

// Retrieve is used by the component to take a request from the incoming buffer
func (p *LimitNumReqPort) Retrieve(now VTimeInSec) Req {
	p.Lock()

	if len(p.Buf) == 0 {
		p.Unlock()
		return nil
	}

	req := p.Buf[0]
	p.Buf = p.Buf[1:]

	if p.PortBusy == true {
		p.PortBusy = false
		p.Unlock()
		p.Conn.NotifyAvailable(now, p)
		return req
	}

	p.Unlock()
	return req
}

// Peek returns the first request in the port without removing it.
func (p *LimitNumReqPort) Peek() Req {
	p.Lock()
	if len(p.Buf) == 0 {
		p.Unlock()
		return nil
	}
	req := p.Buf[0]
	p.Unlock()
	return req
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *LimitNumReqPort) NotifyAvailable(now VTimeInSec) {
	p.Lock()
	p.ConnBusy = false
	p.Unlock()

	if p.Comp != nil {
		p.Comp.NotifyPortFree(now, p)
	}
}

// NewLimitNumReqPort creates a new port that works for the provided component
func NewLimitNumReqPort(comp Component, capacity int) *LimitNumReqPort {
	p := new(LimitNumReqPort)
	p.Comp = comp
	p.BufCapacity = capacity
	return p
}
