// Package cache is a middle level of the tasktree example's hierarchy: a
// ticking component that misses on every read and forwards it to the level
// below. The downstream req_out task is parented to the req_in task the
// cache is handling, which chains the levels into one task tree.
package cache

import (
	"github.com/sarchlab/akita/v5/examples/tasktree/memory"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec is the cache's configuration.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// Downstream is the port misses are forwarded to: the Top port of the
	// level below.
	Downstream messaging.RemotePort `json:"downstream"`
}

// txn pairs a request from above with the request the cache sent below.
type txn struct {
	UpReq   messaging.Msg `json:"up_req"`
	DownReq messaging.Msg `json:"down_req"`
}

// State is the cache's runtime data.
type State struct {
	// Txns holds the forwarded requests, keyed by downstream request ID.
	Txns map[uint64]txn `json:"txns"`
}

// Ports holds the cache's ports.
type Ports struct {
	// Top receives reads from the level above and returns responses.
	Top messaging.Port

	// Bottom forwards reads to the level below and receives responses.
	Bottom messaging.Port
}

// Middlewares holds the cache's behavior.
type Middlewares struct {
	// Forward sends misses down and responses up.
	Forward *forwardMW
}

// Comp is the cache, a ticking component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the cache.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(_ *Comp) State {
	return State{Txns: map[uint64]txn{}}
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Forward: &forwardMW{comp: c}}
}

type forwardMW struct {
	comp *Comp
}

func (m *forwardMW) Handle(_ timing.Event) bool {
	p := false
	p = m.forwardDown() || p
	p = m.respondUp() || p
	return p
}

// forwardDown takes an upstream request and, on a "miss", initiates a child
// request to the next level down.
func (m *forwardMW) forwardDown() bool {
	top, bottom := m.comp.Ports.Top, m.comp.Ports.Bottom

	if !bottom.CanSend() {
		return false
	}
	msg, ok := top.PeekIncoming()
	if !ok {
		return false
	}
	_ = msg.Payload.(memory.ReadReq)
	upReq := msg

	// Open the handling task for the request we received.
	tracing.TraceReqReceive(m.comp, upReq) // req_in @ this cache

	// Miss: send a request one level down, parented to the task above.
	downReq := memory.NewReq(m.comp.NewID(),
		bottom.AsRemote(), m.comp.Spec.Downstream)
	tracing.TraceReqInitiate(m.comp, downReq,
		tracing.MsgIDAtReceiver(upReq, m.comp))
	bottom.Send(downReq)

	m.comp.State.Txns[downReq.ID] = txn{UpReq: upReq, DownReq: downReq}
	top.RetrieveIncoming()
	return true
}

// respondUp takes a downstream response and answers the original requester.
func (m *forwardMW) respondUp() bool {
	top, bottom := m.comp.Ports.Top, m.comp.Ports.Bottom

	if !top.CanSend() {
		return false
	}
	msg, ok := bottom.PeekIncoming()
	if !ok {
		return false
	}
	_ = msg.Payload.(memory.ReadRsp)
	downRsp := msg
	t := m.comp.State.Txns[downRsp.RspTo]

	tracing.TraceReqFinalize(m.comp, t.DownReq) // close the downstream task

	top.Send(memory.NewRsp(m.comp.NewID(),
		top.AsRemote(), t.UpReq.Src, t.UpReq.ID))
	tracing.TraceReqComplete(m.comp, t.UpReq) // close the handling task

	delete(m.comp.State.Txns, downRsp.RspTo)
	bottom.RetrieveIncoming()
	return true
}
