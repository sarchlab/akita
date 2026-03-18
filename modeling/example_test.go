package modeling_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/noc/directconnection"
)

// --- Concrete message types ---

type PingReq struct {
	sim.MsgMeta
	SeqID int
}

type PingRsp struct {
	sim.MsgMeta
	SeqID int
}

// --- Spec and State ---

type PingSpec struct {
	NumPingsToSend int `json:"num_pings_to_send"`
}

type PingState struct {
	NumPingNeedToSend int              `json:"num_ping_need_to_send"`
	NextSeqID         int              `json:"next_seq_id"`
	StartTimes        []sim.VTimeInSec `json:"start_times"`
	CompletedPings    int              `json:"completed_pings"`
}

type pingTransaction struct {
	req       *PingReq
	cycleLeft int
}

// --- Middleware ---

type pingMiddleware struct {
	comp                *modeling.Component[PingSpec, PingState]
	outPort             sim.Port
	pingDst             sim.RemotePort
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
		state := m.comp.GetState()
		state.CompletedPings++
		seqID := msg.SeqID
		startTime := state.StartTimes[seqID]
		currentTime := m.comp.CurrentTime()
		duration := currentTime - startTime

		fmt.Printf("Ping %d, %d\n", seqID, duration)
		m.comp.SetState(state)
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
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.outPort.AsRemote(),
			Dst: trans.req.Src,
		},
		SeqID: trans.req.SeqID,
	}

	err := m.outPort.Send(rsp)
	if err != nil {
		return false
	}

	m.currentTransactions = m.currentTransactions[1:]
	return true
}

func (m *pingMiddleware) sendPing() bool {
	state := m.comp.GetState()
	if state.NumPingNeedToSend == 0 {
		return false
	}

	req := &PingReq{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.outPort.AsRemote(),
			Dst: m.pingDst,
		},
		SeqID: state.NextSeqID,
	}

	err := m.outPort.Send(req)
	if err != nil {
		return false
	}

	state.StartTimes = append(state.StartTimes, m.comp.CurrentTime())
	state.NumPingNeedToSend--
	state.NextSeqID++
	m.comp.SetState(state)

	return true
}

// Example demonstrates building a ping-pong simulation using
// modeling.Component with Spec and State.
func Example() {
	engine := sim.NewSerialEngine()

	specA := PingSpec{NumPingsToSend: 2}
	specB := PingSpec{NumPingsToSend: 0}

	portA := sim.NewPort(nil, 4, 4, "AgentA.OutPort")
	portB := sim.NewPort(nil, 4, 4, "AgentB.OutPort")

	agentA := modeling.NewBuilder[PingSpec, PingState]().
		WithEngine(engine).
		WithFreq(1 * sim.Hz).
		WithSpec(specA).
		Build("AgentA")

	mwA := &pingMiddleware{
		comp:    agentA,
		outPort: portA,
		pingDst: portB.AsRemote(),
	}
	agentA.AddMiddleware(mwA)
	portA.SetComponent(agentA)

	agentB := modeling.NewBuilder[PingSpec, PingState]().
		WithEngine(engine).
		WithFreq(1 * sim.Hz).
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
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	conn.PlugIn(portA)
	conn.PlugIn(portB)

	// Initialize state for agent A.
	stateA := PingState{
		NumPingNeedToSend: specA.NumPingsToSend,
	}
	agentA.SetState(stateA)

	agentA.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5000000000000
	// Ping 1, 5000000000000
}
