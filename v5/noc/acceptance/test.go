package acceptance

import (
	"log"
	"math/rand"

	"github.com/sarchlab/akita/v5/sim"
)

// TrafficMsg is a concrete message type used in acceptance tests.
type TrafficMsg struct {
	sim.MsgMeta
}

// Test is a test case.
type Test struct {
	agents            []*Agent
	msgs              []*TrafficMsg
	receivedMsgs      []*TrafficMsg
	receivedMsgsTable map[string]bool
}

// NewTest creates a new test.
func NewTest() *Test {
	t := &Test{}
	t.receivedMsgsTable = make(map[string]bool)

	return t
}

// RegisterAgent adds an agent to the Test
func (t *Test) RegisterAgent(agent *Agent) {
	t.agents = append(t.agents, agent)
}

// GenerateMsgs generates n message from a random source port to a random
// destination port.
func (t *Test) GenerateMsgs(n uint64) {
	for i := uint64(0); i < n; i++ {
		srcAgentID := rand.Intn(len(t.agents))
		srcAgent := t.agents[srcAgentID]
		srcPortID := rand.Intn(len(srcAgent.AgentPorts))
		srcPort := srcAgent.AgentPorts[srcPortID]

		dstAgentID := rand.Intn(len(t.agents))
		for dstAgentID == srcAgentID {
			dstAgentID = rand.Intn(len(t.agents))
		}

		dstAgent := t.agents[dstAgentID]
		dstPortID := rand.Intn(len(dstAgent.AgentPorts))
		dstPort := dstAgent.AgentPorts[dstPortID]

		msg := &TrafficMsg{
			MsgMeta: sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				Src:          srcPort.AsRemote(),
				Dst:          dstPort.AsRemote(),
				TrafficBytes: rand.Intn(4096),
			},
		}
		srcAgent.MsgsToSend = append(srcAgent.MsgsToSend, msg)
		t.registerMsg(msg)
	}
}

func (t *Test) registerMsg(msg *TrafficMsg) {
	t.msgs = append(t.msgs, msg)
}

// receiveMsgMeta marks that a message (identified by its MsgMeta) is received.
func (t *Test) receiveMsgMeta(meta *sim.MsgMeta, recvPort sim.Port) {
	if meta.Dst != recvPort.AsRemote() {
		panic("msg delivered to a wrong destination")
	}

	if _, found := t.receivedMsgsTable[meta.ID]; found {
		panic("msg is double delivered")
	}
	t.receivedMsgsTable[meta.ID] = true

	// Wrap in TrafficMsg so existing bookkeeping works.
	t.receivedMsgs = append(t.receivedMsgs, &TrafficMsg{MsgMeta: *meta})
}

// MustHaveReceivedAllMsgs asserts that all the messages sent are received.
func (t *Test) MustHaveReceivedAllMsgs() {
	if len(t.msgs) == len(t.receivedMsgs) {
		return
	}

	for _, sentMsg := range t.msgs {
		if _, found := t.receivedMsgsTable[sentMsg.ID]; !found {
			log.Printf("msg %s expected, but not received\n", sentMsg.ID)
		}
	}

	panic("some messages are dropped")
}

// ReportBandwidthAchieved dumps the bandwidth observed by each agents.
func (t *Test) ReportBandwidthAchieved(now sim.VTimeInSec) {
	for _, a := range t.agents {
		log.Printf(
			"agent %s, send bandwidth %.2f GB/s, recv bandwidth %.2f GB/s",
			a.Name(),
			float64(a.sendBytes)/float64(now)/1e9,
			float64(a.recvBytes)/float64(now)/1e9)
	}
}
