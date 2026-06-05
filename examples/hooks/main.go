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

type agentSpec struct {
	Freq timing.Freq `json:"freq"`
}

type pendingRsp struct {
	SeqID int                  `json:"seq_id"`
	ReqID uint64               `json:"req_id"`
	Dst   messaging.RemotePort `json:"dst"`
}

type agentState struct {
	PingsToSend int                  `json:"pings_to_send"`
	NextSeqID   int                  `json:"next_seq_id"`
	PingDst     messaging.RemotePort `json:"ping_dst"`
	Pending     []pendingRsp         `json:"pending"`
}

// Comp is the ping agent. Both agents use this same type.
type Comp = modeling.Component[agentSpec, agentState, modeling.None]

func out(c *Comp) messaging.Port { return c.GetPortByName("Out") }

type agentMW struct {
	comp *Comp
}

func (m *agentMW) Tick() bool {
	progress := false
	progress = m.send() || progress
	progress = m.recv() || progress
	return progress
}

func (m *agentMW) send() bool {
	s := &m.comp.State
	port := out(m.comp)
	progress := false

	if len(s.Pending) > 0 && port.CanSend() {
		p := s.Pending[0]
		port.Send(pingRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   port.AsRemote(),
				Dst:   p.Dst,
				RspTo: p.ReqID,
			},
			SeqID: p.SeqID,
		})
		s.Pending = s.Pending[1:]
		progress = true
	}

	if s.PingsToSend > 0 && port.CanSend() {
		port.Send(pingReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: port.AsRemote(),
				Dst: s.PingDst,
			},
			SeqID: s.NextSeqID,
		})
		s.PingsToSend--
		s.NextSeqID++
		progress = true
	}

	return progress
}

func (m *agentMW) recv() bool {
	port := out(m.comp)
	msgI := port.PeekIncoming()
	if msgI == nil {
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

func buildAgent(reg modeling.Registrar, name string) *Comp {
	c := modeling.NewBuilder[agentSpec, agentState, modeling.None]().
		WithEngine(reg.GetEngine()).
		WithFreq(1 * timing.GHz).
		WithSpec(agentSpec{Freq: 1 * timing.GHz}).
		Build(name)
	c.AddMiddleware(&agentMW{comp: c})
	c.AddPort("Out", messaging.NewPort(c, 4, 4, name+".Out"))
	reg.RegisterComponent(c)
	return c
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
	registrar := modeling.NewStandaloneRegistrar(engine)

	agentA := buildAgent(registrar, "AgentA")
	agentB := buildAgent(registrar, "AgentB")

	conn := directconnection.MakeBuilder().
		WithRegistrar(registrar).
		Build("Conn")
	conn.PlugIn(agentA.GetPortByName("Out"))
	conn.PlugIn(agentB.GetPortByName("Out"))

	// Attach the hooks. The agents above never reference these.
	engine.AcceptHook(&eventHook{})
	agentA.GetPortByName("Out").AcceptHook(&msgHook{agent: "AgentA"})
	agentB.GetPortByName("Out").AcceptHook(&msgHook{agent: "AgentB"})

	state := agentA.State
	state.PingDst = agentB.GetPortByName("Out").AsRemote()
	state.PingsToSend = 1
	agentA.State = state

	agentA.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}
}
