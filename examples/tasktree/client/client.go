// Package client is the top of the tasktree example's hierarchy: a ticking
// component that sends NumReqs reads to Dst, one at a time. Each read's
// req_out task has no parent, so it is the root of a task tree.
package client

import (
	"github.com/sarchlab/akita/v5/examples/tasktree/memory"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec is the client's configuration.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// Dst is the port the client sends its reads to.
	Dst messaging.RemotePort `json:"dst"`

	// NumReqs is the number of reads the client sends.
	NumReqs int `json:"num_reqs"`
}

// State is the client's runtime data.
type State struct {
	// Sent counts the reads sent so far.
	Sent int `json:"sent"`

	// InFlight holds the reads awaiting a response, keyed by ID.
	InFlight map[uint64]memory.ReadReq `json:"in_flight"`
}

// Ports holds the client's only port.
type Ports struct {
	// Out sends reads and receives responses.
	Out messaging.Port
}

// Middlewares holds the client's behavior.
type Middlewares struct {
	// Request takes responses and sends the next read.
	Request *requestMW
}

// Comp is the client, a ticking component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the client.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz, NumReqs: 1},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(_ *Comp) State {
	return State{InFlight: map[uint64]memory.ReadReq{}}
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Request: &requestMW{comp: c}}
}

type requestMW struct {
	comp *Comp
}

func (m *requestMW) Handle(_ timing.Event) bool {
	p := false
	p = m.receive() || p
	p = m.send() || p
	return p
}

func (m *requestMW) send() bool {
	s := &m.comp.State
	spec := m.comp.Spec
	port := m.comp.Ports.Out
	if s.Sent == spec.NumReqs || len(s.InFlight) > 0 || !port.CanSend() {
		return false
	}

	req := memory.NewReq(m.comp.NewID(), port.AsRemote(), spec.Dst)
	tracing.TraceReqInitiate(m.comp, req, 0) // root task, no parent
	port.Send(req)
	s.InFlight[req.ID] = req
	s.Sent++
	return true
}

func (m *requestMW) receive() bool {
	s := &m.comp.State
	port := m.comp.Ports.Out
	msg, ok := port.PeekIncoming()
	if !ok {
		return false
	}
	rsp := msg.(memory.ReadRsp)
	if req, ok := s.InFlight[rsp.RspTo]; ok {
		tracing.TraceReqFinalize(m.comp, req)
		delete(s.InFlight, rsp.RspTo)
	}
	port.RetrieveIncoming()
	return true
}
