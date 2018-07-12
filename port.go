package core

import "sync"

// A Port is owned by a component and is used to plugin connections
type Port struct {
	sync.Mutex

	Buf         []Req
	BufCapacity int
	PortBusy    bool

	Conn     Connection
	ConnBusy bool

	Comp Component
}

// Send is used to send a request out from a component
func (p *Port) Send(req Req) *SendError {
	err := p.Conn.Send(req)
	if err != nil {
		p.Lock()
		p.ConnBusy = true
		p.Unlock()
	}
	return err
}

// Recv is used to deliver a request to a component
func (p *Port) Recv(req Req) *SendError {
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
func (p *Port) Retrieve(now VTimeInSec) Req {
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
func (p *Port) Peek() Req {
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
func (p *Port) NotifyAvailable(now VTimeInSec) {
	p.ConnBusy = false
	p.Comp.NotifyPortFree(now, p)
}

// NewPort creates a new port that works for the provided component
func NewPort(comp Component) *Port {
	p := new(Port)
	p.Comp = comp
	p.BufCapacity = 4
	return p
}
