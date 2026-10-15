// Package client is the requesting side of the reqtracing example: a ticking
// component that sends NumReqs requests to Dst, one at a time. It traces each
// round trip as a req_out task, opened with StartReqOut and closed with
// EndReqOut.
package client

import (
	"github.com/sarchlab/akita/v5/examples/reqtracing/server"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
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

	// InFlight holds task IDs for requests awaiting a response, keyed by message ID.
	InFlight map[uint64]uint64 `json:"in_flight"`
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
	DefaultSpec: Spec{Freq: 1 * timing.GHz, NumReqs: 1},
}

func (initialState *State) Initialize(_ *Comp) {
	*initialState = State{InFlight: map[uint64]uint64{}}
}

func (initialMiddlewares *Middlewares) Initialize(c *Comp) {
	*initialMiddlewares = Middlewares{Request: &requestMW{comp: c}}
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

	// Store the returned task ID, separately from the request ID.
	taskID := tasks.StartReqOut(m.comp, req, 0)
	port.Send(req)

	s.InFlight[req.ID] = taskID
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
	if taskID, ok := s.InFlight[rsp.RspTo]; ok {
		tasks.EndReqOut(m.comp, taskID)
		delete(s.InFlight, rsp.RspTo)
	}
	port.RetrieveIncoming()

	return true
}
