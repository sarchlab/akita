// Package server is the responding side of the reqtracing example: a ticking
// component that answers every ReadReq Latency cycles after it arrives. It
// traces its handling of each request as a req_in task, opened with
// TraceReqReceive and closed with TraceReqComplete.
package server

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

// ReadReq asks the server for a read.
type ReadReq struct {
	Seq int
}

// ReadRsp answers the ReadReq whose ID is RspTo.
type ReadRsp struct {
	Seq int
}

// Spec is the server's configuration.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// Latency is the number of cycles the server takes to handle a request.
	Latency int `json:"latency"`
}

// txn is a request being handled. Left counts the cycles until it is done.
type txn struct {
	Req  messaging.Msg `json:"req"`
	Left int           `json:"left"`
}

// State is the server's runtime data.
type State struct {
	Pending []txn `json:"pending"`
}

// Ports holds the server's only port.
type Ports struct {
	// Out receives requests and sends responses.
	Out messaging.Port
}

// Middlewares holds the server's behavior.
type Middlewares struct {
	// Serve takes requests, counts down their latency, and responds in order.
	Serve *serveMW
}

// Comp is the server, a ticking component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the server.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz, Latency: 4},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Serve: &serveMW{comp: c}}
}

type serveMW struct {
	comp *Comp
}

func (m *serveMW) Handle(_ timing.Event) bool {
	progress := false
	progress = m.respond() || progress
	progress = m.countDown() || progress
	progress = m.receive() || progress
	return progress
}

func (m *serveMW) receive() bool {
	port := m.comp.Ports.Out

	msg, ok := port.PeekIncoming()
	if !ok {
		return false
	}

	_ = msg.Payload.(ReadReq)
	req := msg
	tracing.TraceReqReceive(m.comp, req)
	m.comp.State.Pending = append(m.comp.State.Pending,
		txn{Req: req, Left: m.comp.Spec.Latency})
	port.RetrieveIncoming()

	return true
}

func (m *serveMW) countDown() bool {
	pending := m.comp.State.Pending
	progress := false

	for i := range pending {
		if pending[i].Left > 0 {
			pending[i].Left--
			progress = true
		}
	}

	return progress
}

func (m *serveMW) respond() bool {
	s := &m.comp.State
	if len(s.Pending) == 0 || s.Pending[0].Left > 0 {
		return false
	}

	port := m.comp.Ports.Out
	if !port.CanSend() {
		return false
	}

	req := s.Pending[0].Req
	port.Send(messaging.Msg{ID: m.comp.NewID(),
		Src:   port.AsRemote(),
		Dst:   req.Src,
		RspTo: req.ID, Payload: ReadRsp{
			Seq: req.Payload.(ReadReq).Seq,
		}})

	tracing.TraceReqComplete(m.comp, req)
	s.Pending = s.Pending[1:]

	return true
}

var _ = messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{ReadReq{}, ReadRsp{}}})
