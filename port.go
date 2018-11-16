package akita

import (
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

type portImpl struct {
	sync.Mutex

	Buf         []Req
	BufCapacity int
	PortBusy    bool

	Conn     Connection
	ConnBusy bool

	Comp Component
}

func (p *portImpl) SetConnection(conn Connection) {
	p.Conn = conn
}

func (p *portImpl) Component() Component {
	return p.Comp
}

// Send is used to send a request out from a component
func (p *portImpl) Send(req Req) *SendError {
	err := p.Conn.Send(req)
	if err != nil {
		p.Lock()
		p.ConnBusy = true
		p.Unlock()
	}
	return err
}

// Recv is used to deliver a request to a component
func (p *portImpl) Recv(req Req) *SendError {
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
func (p *portImpl) Retrieve(now VTimeInSec) Req {
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
func (p *portImpl) Peek() Req {
	p.Lock()
	if len(p.Buf) == 0 {
		p.Unlock()
		return nil
	}
	p.Unlock()
	return p.Buf[0]
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *portImpl) NotifyAvailable(now VTimeInSec) {
	p.Lock()
	p.ConnBusy = false
	p.Unlock()

	if p.Comp != nil {
		p.Comp.NotifyPortFree(now, p)
	}
}

// NewPort creates a new port that works for the provided component
func NewPort(comp Component) Port {
	p := new(portImpl)
	p.Comp = comp
	p.BufCapacity = 1
	return p
}
