package modeling_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"
)

// --- Payload types ---

type PingReqPayload struct {
	SeqID int
}

type PingRspPayload struct {
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
	req       *sim.Msg
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
	msg := m.outPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch payload := msg.Payload.(type) {
	case *PingReqPayload:
		_ = payload
		trans := &pingTransaction{req: msg, cycleLeft: 2}
		m.currentTransactions = append(m.currentTransactions, trans)
		m.outPort.RetrieveIncoming()
	case *PingRspPayload:
		state := m.comp.GetState()
		state.CompletedPings++
		seqID := payload.SeqID
		startTime := state.StartTimes[seqID]
		currentTime := m.comp.CurrentTime()
		duration := currentTime - startTime

		fmt.Printf("Ping %d, %.2f\n", seqID, duration)
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

	reqPayload := trans.req.Payload.(*PingReqPayload)
	rsp := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.outPort.AsRemote(),
			Dst: trans.req.Src,
		},
		Payload: &PingRspPayload{SeqID: reqPayload.SeqID},
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

	req := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.outPort.AsRemote(),
			Dst: m.pingDst,
		},
		Payload: &PingReqPayload{SeqID: state.NextSeqID},
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
	// Ping 0, 5.00
	// Ping 1, 5.00
}
