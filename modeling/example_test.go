package modeling_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"

	// --- Concrete message types ---
	"github.com/sarchlab/akita/v5/messaging"
)

type PingReq struct {
	messaging.MsgMeta
	SeqID int
}

type PingRsp struct {
	messaging.MsgMeta
	SeqID int
}

// --- Spec and State ---

type PingSpec struct {
	NumPingsToSend int `json:"num_pings_to_send"`
}

type PingState struct {
	NumPingNeedToSend int                 `json:"num_ping_need_to_send"`
	NextSeqID         int                 `json:"next_seq_id"`
	StartTimes        []timing.VTimeInPicoSec `json:"start_times"`
	CompletedPings    int                 `json:"completed_pings"`
}

type pingTransaction struct {
	req       *PingReq
	cycleLeft int
}

// --- Middleware ---

type pingMiddleware struct {
	comp                *modeling.Component[PingSpec, PingState, modeling.None]
	outPort             messaging.Port
	pingDst             messaging.RemotePort
	currentTransactions []*pingTransaction
}

func (m *pingMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendRsp() || madeProgress
	madeProgress = m.sendPing() || madeProgress
	madeProgress = m.countDown() || madeProgress
	madeProgress = m.processInput() || madeProgress

	return madeProgress
}

func (m *pingMiddleware) processInput() bool {
	rawMsg := m.outPort.PeekIncoming()
	if rawMsg == nil {
		return false
	}

	switch msg := rawMsg.(type) {
	case *PingReq:
		trans := &pingTransaction{req: msg, cycleLeft: 2}
		m.currentTransactions = append(m.currentTransactions, trans)
		m.outPort.RetrieveIncoming()
	case *PingRsp:
		state := m.comp.State
		state.CompletedPings++
		seqID := msg.SeqID
		startTime := state.StartTimes[seqID]
		currentTime := m.comp.CurrentTime()
		duration := currentTime - startTime

		fmt.Printf("Ping %d, %d\n", seqID, duration)
		m.comp.State = state
		m.outPort.RetrieveIncoming()
	default:
		panic("unknown message type")
	}

	return true
}

func (m *pingMiddleware) countDown() bool {
	madeProgress := false
	for _, trans := range m.currentTransactions {
		if trans.cycleLeft > 0 {
			trans.cycleLeft--
			madeProgress = true
		}
	}
	return madeProgress
}

func (m *pingMiddleware) sendRsp() bool {
	if len(m.currentTransactions) == 0 {
		return false
	}

	trans := m.currentTransactions[0]
	if trans.cycleLeft > 0 {
		return false
	}

	rsp := &PingRsp{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: m.outPort.AsRemote(),
			Dst: trans.req.Src,
		},
		SeqID: trans.req.SeqID,
	}

	if !m.outPort.CanSend() {
		return false
	}

	m.outPort.Send(rsp)

	m.currentTransactions = m.currentTransactions[1:]
	return true
}

func (m *pingMiddleware) sendPing() bool {
	state := m.comp.State
	if state.NumPingNeedToSend == 0 {
		return false
	}

	req := &PingReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: m.outPort.AsRemote(),
			Dst: m.pingDst,
		},
		SeqID: state.NextSeqID,
	}

	if !m.outPort.CanSend() {
		return false
	}

	m.outPort.Send(req)

	state.StartTimes = append(state.StartTimes, m.comp.CurrentTime())
	state.NumPingNeedToSend--
	state.NextSeqID++
	m.comp.State = state

	return true
}

// Example demonstrates building a ping-pong simulation using
// modeling.Component with Spec and State.
func Example() {
	engine := timing.NewSerialEngine()

	specA := PingSpec{NumPingsToSend: 2}
	specB := PingSpec{NumPingsToSend: 0}

	portA := messaging.NewPort(nil, 4, 4, "AgentA.OutPort")
	portB := messaging.NewPort(nil, 4, 4, "AgentB.OutPort")

	agentA := modeling.NewBuilder[PingSpec, PingState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.Hz).
		WithSpec(specA).
		Build("AgentA")

	mwA := &pingMiddleware{
		comp:    agentA,
		outPort: portA,
		pingDst: portB.AsRemote(),
	}
	agentA.AddMiddleware(mwA)
	portA.SetComponent(agentA)

	agentB := modeling.NewBuilder[PingSpec, PingState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.Hz).
		WithSpec(specB).
		Build("AgentB")

	mwB := &pingMiddleware{
		comp:    agentB,
		outPort: portB,
		pingDst: portA.AsRemote(),
	}
	agentB.AddMiddleware(mwB)
	portB.SetComponent(agentB)

	conn := directconnection.
		MakeBuilder().
		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
		Build("Conn")

	conn.PlugIn(portA)
	conn.PlugIn(portB)

	// Initialize state for agent A.
	stateA := PingState{
		NumPingNeedToSend: specA.NumPingsToSend,
	}
	agentA.State = stateA

	agentA.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5000000000000
	// Ping 1, 5000000000000
}
