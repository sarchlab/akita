// Package memory is the leaf of the tasktree example's hierarchy: a ticking
// component that answers every read at once. Its req_in task has no children.
//
// The package also defines the read request and response that every level
// of the hierarchy speaks.
package memory

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// ReadReq asks the level below for a read.
type ReadReq struct {
	messaging.MsgMeta
}

// ReadRsp answers the ReadReq whose ID is RspTo.
type ReadRsp struct {
	messaging.MsgMeta
}

// NewReq returns a read request with the given ID from src to dst.
func NewReq(id uint64, src, dst messaging.RemotePort) ReadReq {
	return ReadReq{MsgMeta: messaging.MsgMeta{ID: id, Src: src, Dst: dst}}
}

// NewRsp returns a response with the given ID from src to dst, for the
// request whose ID is rspTo.
func NewRsp(id uint64, src, dst messaging.RemotePort, rspTo uint64) ReadRsp {
	return ReadRsp{MsgMeta: messaging.MsgMeta{
		ID: id, Src: src, Dst: dst, RspTo: rspTo}}
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
	req := msg.(ReadReq)

	tracing.TraceReqReceive(m.comp, req) // req_in @ Memory — a leaf task
	port.Send(NewRsp(m.comp.NewID(), port.AsRemote(), req.Src, req.ID))
	tracing.TraceReqComplete(m.comp, req)
	port.RetrieveIncoming()
	return true
}
