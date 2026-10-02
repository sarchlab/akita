package directconnection

import (
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
	gomock "go.uber.org/mock/gomock"
)

type testMsg struct {
	messaging.MsgMeta
}

func newTestMsg(sim timing.Simulation) testMsg {
	return testMsg{
		MsgMeta: messaging.MsgMeta{
			ID: sim.NewID(),
		},
	}
}

var _ = Describe("DirectConnection", func() {

	var (
		mockCtrl   *gomock.Controller
		port1      *MockPort
		port2      *MockPort
		engine     *MockEngine
		sim        timing.Simulation
		connection *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		port1 = NewMockPort(mockCtrl)
		port1.EXPECT().AsRemote().Return(messaging.RemotePort("port1")).AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().AsRemote().Return(messaging.RemotePort("port2")).AnyTimes()

		engine = NewMockEngine(mockCtrl)
		sim = modeling.NewStandaloneSimulation(engine)
		connection = MakeBuilder().
			WithSimulation(sim).
			Build("Direct")

		port1.EXPECT().SetConnection(connection)
		connection.PlugIn(port1)

		port2.EXPECT().SetConnection(connection)
		connection.PlugIn(port2)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should forward when handling tick event", func() {
		engine.EXPECT().CurrentTime().Return(timing.VTimeInPicoSec(10000))

		tick := ticking.MakeTickEvent(sim.NewID(), connection.Name(), timing.VTimeInPicoSec(10000))

		msg1 := newTestMsg(sim)
		msg1.Src = port1.AsRemote()
		msg1.Dst = port2.AsRemote()

		msg2 := newTestMsg(sim)
		msg2.Src = port2.AsRemote()
		msg2.Dst = port1.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg1, true)
		port1.EXPECT().PeekOutgoing().Return(nil, false)
		port1.EXPECT().RetrieveOutgoing().Return(msg1, true)
		port1.EXPECT().CanDeliver().Return(true)
		port1.EXPECT().Deliver(msg2)

		port2.EXPECT().PeekOutgoing().Return(msg2, true)
		port2.EXPECT().PeekOutgoing().Return(nil, false)
		port2.EXPECT().RetrieveOutgoing().Return(msg2, true)
		port2.EXPECT().CanDeliver().Return(true)
		port2.EXPECT().Deliver(msg1)

		engine.EXPECT().
			Schedule(gomock.Any()).
			Do(func(evt ticking.TickEvent) {
				Expect(evt.Time()).To(Equal(timing.VTimeInPicoSec(11000)))
				Expect(evt.IsSecondary()).To(BeTrue())
			})

		connection.Handle(tick)
	})

	It("should keep outgoing messages queued when delivery is blocked", func() {
		tick := ticking.MakeTickEvent(sim.NewID(), connection.Name(), timing.VTimeInPicoSec(10000))

		msg := newTestMsg(sim)
		msg.Src = port1.AsRemote()
		msg.Dst = port2.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg, true)
		port2.EXPECT().CanDeliver().Return(false)
		port2.EXPECT().PeekOutgoing().Return(nil, false)

		connection.Handle(tick)
	})
})

// agent is a test double that sends its messages out of OutPort and records
// what it receives, ticking while it makes progress.
type agent struct {
	*ticking.Scheduler
	hooking.HookableBase

	name    string
	msgsOut []testMsg
	msgsIn  []messaging.Msg

	OutPort messaging.Port
}

func newAgent(sim timing.Simulation, freq timing.Freq, name string, outPort messaging.Port) *agent {
	a := &agent{
		Scheduler: ticking.NewScheduler(name, sim, freq),
		name:      name,
		OutPort:   outPort,
	}
	a.OutPort.SetOwner(a)
	sim.Engine().(timing.HandlerRegistry).RegisterHandler(name, a)

	return a
}

func (a *agent) Name() string                  { return a.name }
func (a *agent) NotifyRecv(messaging.Port)     { a.TickLater() }
func (a *agent) NotifyPortFree(messaging.Port) { a.TickLater() }

func (a *agent) Handle(timing.Event) {
	madeProgress := false

	msgIn, ok := a.OutPort.RetrieveIncoming()
	if ok {
		a.msgsIn = append(a.msgsIn, msgIn)
		madeProgress = true
	}

	if len(a.msgsOut) > 0 && a.OutPort.CanSend() {
		a.OutPort.Send(a.msgsOut[0])
		madeProgress = true
		a.msgsOut = a.msgsOut[1:]
	}

	if madeProgress {
		a.TickLater()
	}
}

var _ = Describe("Direct Connection Integration", func() {
	var (
		mockCtrl        *gomock.Controller
		engine          timing.Engine
		sim             timing.Simulation
		connection      *Comp
		agents          []*agent
		numAgents       = 10
		numMsgsPerAgent = 1000
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		connection = MakeBuilder().
			WithSimulation(sim).
			Build("Conn")
		agents = nil
		for i := 0; i < numAgents; i++ {
			a := newAgent(sim, 1*timing.GHz, fmt.Sprintf("Agent[%d]", i),
				messaging.NewPort(fmt.Sprintf("Agent[%d].OutPort", i), 4, 4))
			agents = append(agents, a)
			connection.PlugIn(a.OutPort)
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should deliver all messages", func() {
		for _, agent := range agents {
			for i := 0; i < numMsgsPerAgent; i++ {
				msg := newTestMsg(sim)
				msg.Src = agent.OutPort.AsRemote()
				msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				for msg.Dst == msg.Src {
					msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				}
				msg.ID = sim.NewID()
				agent.msgsOut = append(agent.msgsOut, msg)
			}
			agent.TickLater()
		}

		Expect(engine.Run()).To(Succeed())

		totalRecvedMsgCount := 0
		for _, agent := range agents {
			totalRecvedMsgCount += len(agent.msgsIn)
		}

		Expect(totalRecvedMsgCount).To(Equal(numAgents * numMsgsPerAgent))
	})

	It("should run deterministicly", func() {
		seed := time.Now().UTC().UnixNano()
		time1 := directConnectionTest(seed)
		time2 := directConnectionTest(seed)

		Expect(time1).To(Equal(time2))
	})
})

func directConnectionTest(seed int64) timing.VTimeInPicoSec {
	r := rand.New(rand.NewSource(seed))

	numAgents := 100
	numMsgsPerAgent := 1000
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	connection := MakeBuilder().
		WithSimulation(sim).
		Build("Conn")
	agents := make([]*agent, 0, numAgents)

	for i := 0; i < numAgents; i++ {
		a := newAgent(sim, 1*timing.GHz, fmt.Sprintf("Agent%d", i),
			messaging.NewPort(fmt.Sprintf("Agent%d.OutPort", i), 4, 4))
		agents = append(agents, a)
		connection.PlugIn(a.OutPort)
	}

	for _, agent := range agents {
		for i := 0; i < numMsgsPerAgent; i++ {
			msg := newTestMsg(sim)
			msg.Src = agent.OutPort.AsRemote()
			msg.Dst = agents[r.Intn(len(agents))].OutPort.AsRemote()

			for msg.Dst == msg.Src {
				msg.Dst = agents[r.Intn(len(agents))].OutPort.AsRemote()
			}

			msg.ID = sim.NewID()

			agent.msgsOut = append(agent.msgsOut, msg)
		}

		agent.TickLater()
	}

	Expect(engine.Run()).To(Succeed())

	return engine.CurrentTime()
}
