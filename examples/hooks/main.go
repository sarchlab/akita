// Command hooks shows how to observe a running simulation with hooks.
//
// Two ticking agents exchange a ping/response over a direct connection. The
// agents themselves print nothing — every line of output comes from two hooks
// attached from the outside: an engine hook that logs each event, and a port
// hook that logs each message. This is the whole point of hooks: you watch a
// simulation without changing the components under test.
package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

// --- Messages ---

type pingReq struct {
	messaging.MsgMeta
	SeqID int
}

type pingRsp struct {
	messaging.MsgMeta
	SeqID int
}

// --- Component ---

// Spec is an agent's configuration. An agent sends NumPings pings to PingDst
// and answers every ping it receives.
type Spec struct {
	Freq     timing.Freq          `json:"freq"`
	PingDst  messaging.RemotePort `json:"ping_dst"`
	NumPings int                  `json:"num_pings"`
}

type pendingRsp struct {
	SeqID int                  `json:"seq_id"`
	ReqID uint64               `json:"req_id"`
	Dst   messaging.RemotePort `json:"dst"`
}

// State is an agent's runtime data.
type State struct {
	NextSeqID int          `json:"next_seq_id"`
	Pending   []pendingRsp `json:"pending"`
}

// Ports holds an agent's only port.
type Ports struct {
	// Out sends pings and responses and receives them from the peer.
	Out messaging.Port
}

// Middlewares holds an agent's behavior.
type Middlewares struct {
	// Agent sends pings and responses and takes incoming messages.
	Agent *agentMW
}

// Comp is the ping agent, a ticking component. Both agents use this type.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the ping agent.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Agent: &agentMW{comp: c}}
}

type agentMW struct {
	comp *Comp
}

func (m *agentMW) Handle(_ timing.Event) bool {
	progress := false
	progress = m.send() || progress
	progress = m.recv() || progress
	return progress
}

func (m *agentMW) send() bool {
	s := &m.comp.State
	spec := m.comp.Spec()
	port := m.comp.Ports.Out
	progress := false

	if len(s.Pending) > 0 && port.CanSend() {
		p := s.Pending[0]
		port.Send(pingRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    m.comp.NewID(),
				Src:   port.AsRemote(),
				Dst:   p.Dst,
				RspTo: p.ReqID,
			},
			SeqID: p.SeqID,
		})
		s.Pending = s.Pending[1:]
		progress = true
	}

	if s.NextSeqID < spec.NumPings && port.CanSend() {
		port.Send(pingReq{
			MsgMeta: messaging.MsgMeta{
				ID:  m.comp.NewID(),
				Src: port.AsRemote(),
				Dst: spec.PingDst,
			},
			SeqID: s.NextSeqID,
		})
		s.NextSeqID++
		progress = true
	}

	return progress
}

func (m *agentMW) recv() bool {
	port := m.comp.Ports.Out
	msgI, ok := port.PeekIncoming()
	if !ok {
		return false
	}

	if req, ok := msgI.(pingReq); ok {
		m.comp.State.Pending = append(m.comp.State.Pending, pendingRsp{
			SeqID: req.SeqID,
			ReqID: req.ID,
			Dst:   req.Src,
		})
	}

	port.RetrieveIncoming()
	return true
}

// --- Hooks ---

// eventHook logs every event the engine is about to handle.
type eventHook struct{}

func (h *eventHook) Func(ctx hooking.HookCtx) {
	if ctx.Pos != timing.HookPosBeforeEvent {
		return
	}
	evt := ctx.Item.(timing.Event)
	fmt.Printf("[event] t=%d handler=%s\n", evt.Time(), evt.HandlerID())
}

// msgHook logs every message that leaves or arrives at the port it is attached
// to. It carries the agent name so the log says who is sending or receiving.
type msgHook struct {
	agent string
}

func (h *msgHook) Func(ctx hooking.HookCtx) {
	msg := ctx.Item.(messaging.Msg)

	switch ctx.Pos {
	case messaging.HookPosPortMsgSend:
		fmt.Printf("[msg]   %s sends %T\n", h.agent, msg)
	case messaging.HookPosPortMsgRecvd:
		fmt.Printf("[msg]   %s recvd %T\n", h.agent, msg)
	}
}

func main() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	// Create the ports first, so AgentA's Spec can name AgentB's port.
	outA := messaging.NewPort("AgentA.Out", 4, 4)
	outB := messaging.NewPort("AgentB.Out", 4, 4)

	agentA := Definition.Builder().
		WithSimulation(sim).
		WithSpec(Spec{Freq: 1 * timing.GHz, PingDst: outB.AsRemote(), NumPings: 1}).
		WithPorts(Ports{Out: outA}).
		Build("AgentA")
	agentB := Definition.Builder().
		WithSimulation(sim).
		WithPorts(Ports{Out: outB}).
		Build("AgentB")

	conn := directconnection.MakeBuilder().
		WithSimulation(sim).
		Build("Conn")
	conn.PlugIn(agentA.Ports.Out)
	conn.PlugIn(agentB.Ports.Out)

	// Attach the hooks. The agents above never reference these.
	engine.AcceptHook(&eventHook{})
	agentA.Ports.Out.AcceptHook(&msgHook{agent: "AgentA"})
	agentB.Ports.Out.AcceptHook(&msgHook{agent: "AgentB"})

	agentA.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}
}
