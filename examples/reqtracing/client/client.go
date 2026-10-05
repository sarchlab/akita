// Package client is the requesting side of the reqtracing example: a ticking
// component that sends NumReqs requests to Dst, one at a time. It traces each
// round trip as a req_out task, opened with TraceReqInitiate and closed with
// TraceReqFinalize.
package client

import (
	"github.com/sarchlab/akita/v5/examples/reqtracing/server"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

// Spec is the client's configuration.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// Dst is the port the client sends its requests to.
	Dst messaging.RemotePort `json:"dst"`

	// NumReqs is the number of requests the client sends.
	NumReqs int `json:"num_reqs"`
}

// State is the client's runtime data.
type State struct {
	// NextSeq numbers the next request, and so counts those sent.
	NextSeq int `json:"next_seq"`

	// InFlight holds the requests awaiting a response, keyed by ID.
	InFlight map[uint64]messaging.Msg `json:"in_flight"`
}

// Ports holds the client's only port.
type Ports struct {
	// Out sends requests and receives responses.
	Out messaging.Port
}

// Middlewares holds the client's behavior.
type Middlewares struct {
	// Request takes responses and sends the next request.
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
	return State{InFlight: map[uint64]messaging.Msg{}}
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Request: &requestMW{comp: c}}
}

type requestMW struct {
	comp *Comp
}

func (m *requestMW) Handle(_ timing.Event) bool {
	progress := false
	progress = m.receive() || progress
	progress = m.send() || progress
	return progress
}

func (m *requestMW) send() bool {
	s := &m.comp.State
	spec := m.comp.Spec
	port := m.comp.Ports.Out

	// Send one request at a time: wait for the response before the next.
	if s.NextSeq == spec.NumReqs || len(s.InFlight) > 0 || !port.CanSend() {
		return false
	}

	req := messaging.Msg{ID: m.comp.NewID(),
		Src: port.AsRemote(),
		Dst: spec.Dst, Payload: server.ReadReq{
			Seq: s.NextSeq,
		}}

	// The req_out task is keyed by the request's own message ID.
	tracing.TraceReqInitiate(m.comp, req, 0)
	port.Send(req)

	s.InFlight[req.ID] = req
	s.NextSeq++

	return true
}

func (m *requestMW) receive() bool {
	s := &m.comp.State
	port := m.comp.Ports.Out

	msg, ok := port.PeekIncoming()
	if !ok {
		return false
	}

	_ = msg.Payload.(server.ReadRsp)
	rsp := msg
	if req, ok := s.InFlight[rsp.RspTo]; ok {
		tracing.TraceReqFinalize(m.comp, req)
		delete(s.InFlight, rsp.RspTo)
	}
	port.RetrieveIncoming()

	return true
}
