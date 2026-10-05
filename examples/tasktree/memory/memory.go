// Package memory is the leaf of the tasktree example's hierarchy: a ticking
// component that answers every read at once. Its req_in task has no children.
//
// The package also defines the read request and response that every level
// of the hierarchy speaks.
package memory

import (
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
	"github.com/sarchlab/akita/v5/simulation/timing"
	"github.com/sarchlab/akita/v5/simulation/tracing"
)

// ReadReq asks the level below for a read.
type ReadReq struct {
}

// ReadRsp answers the ReadReq whose ID is RspTo.
type ReadRsp struct {
}

// NewReq returns a read request with the given ID from src to dst.
func NewReq(id uint64, src, dst messaging.RemotePort) messaging.Msg {
	return messaging.Msg{ID: id, Src: src, Dst: dst, Payload: ReadReq{}}
}

// NewRsp returns a response with the given ID from src to dst, for the
// request whose ID is rspTo.
func NewRsp(id uint64, src, dst messaging.RemotePort, rspTo uint64) messaging.Msg {
	return messaging.Msg{ID: id, Src: src, Dst: dst, RspTo: rspTo, Payload: ReadRsp{}}
}

// Spec is the memory's configuration.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}

// Ports holds the memory's only port.
type Ports struct {
	// Top receives reads from the level above and returns responses.
	Top messaging.Port
}

// Middlewares holds the memory's behavior.
type Middlewares struct {
	// Serve answers one read per tick.
	Serve *serveMW
}

// Comp is the memory, a ticking component. It keeps no State.
type Comp = ticking.Component[Spec, modeling.None, modeling.None, Ports, Middlewares]

// Definition declares the memory.
var Definition = ticking.Definition[Spec, modeling.None, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Serve: &serveMW{comp: c}}
}

type serveMW struct {
	comp *Comp
}

func (m *serveMW) Handle(_ timing.Event) bool {
	port := m.comp.Ports.Top
	msg, ok := port.PeekIncoming()
	if !ok {
		return false
	}
	if !port.CanSend() {
		return false
	}
	_ = msg.Payload.(ReadReq)
	req := msg

	tracing.TraceReqReceive(m.comp, req) // req_in @ Memory — a leaf task
	port.Send(NewRsp(m.comp.NewID(), port.AsRemote(), req.Src, req.ID))
	tracing.TraceReqComplete(m.comp, req)
	port.RetrieveIncoming()
	return true
}

var _ = messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{ReadReq{}, ReadRsp{}}})
