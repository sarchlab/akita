package directconnection

import (
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	gomock "go.uber.org/mock/gomock"
)

type testMsg struct {
	messaging.MsgMeta
}

func newTestMsg() *testMsg {
	return &testMsg{
		MsgMeta: messaging.MsgMeta{
			ID: timing.GetIDGenerator().Generate(),
		},
	}
}

var _ = Describe("DirectConnection", func() {

	var (
		mockCtrl   *gomock.Controller
		port1      *MockPort
		port2      *MockPort
		engine     *MockEngine
		connection *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		port1 = NewMockPort(mockCtrl)
		port1.EXPECT().AsRemote().Return(messaging.RemotePort("port1")).AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().AsRemote().Return(messaging.RemotePort("port2")).AnyTimes()

		engine = NewMockEngine(mockCtrl)
		connection = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
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

		tick := modeling.MakeTickEvent(connection.Name(), timing.VTimeInPicoSec(10000))

		msg1 := newTestMsg()
		msg1.Src = port1.AsRemote()
		msg1.Dst = port2.AsRemote()

		msg2 := newTestMsg()
		msg2.Src = port2.AsRemote()
		msg2.Dst = port1.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg1)
		port1.EXPECT().PeekOutgoing().Return(nil)
		port1.EXPECT().RetrieveOutgoing().Return(msg1)
		port1.EXPECT().CanDeliver().Return(true)
		port1.EXPECT().Deliver(msg2)

		port2.EXPECT().PeekOutgoing().Return(msg2)
		port2.EXPECT().PeekOutgoing().Return(nil)
		port2.EXPECT().RetrieveOutgoing().Return(msg2)
		port2.EXPECT().CanDeliver().Return(true)
		port2.EXPECT().Deliver(msg1)

		engine.EXPECT().
			Schedule(gomock.Any()).
			Do(func(evt modeling.TickEvent) {
				Expect(evt.Time()).To(Equal(timing.VTimeInPicoSec(11000)))
				Expect(evt.IsSecondary()).To(BeTrue())
			})

		connection.Handle(tick)
	})

	It("should keep outgoing messages queued when delivery is blocked", func() {
		tick := modeling.MakeTickEvent(connection.Name(), timing.VTimeInPicoSec(10000))

		msg := newTestMsg()
		msg.Src = port1.AsRemote()
		msg.Dst = port2.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg)
		port2.EXPECT().CanDeliver().Return(false)
		port2.EXPECT().PeekOutgoing().Return(nil)

		connection.Handle(tick)
	})
})

type agent struct {
	*modeling.TickingComponent

	msgsOut []*testMsg
	msgsIn  []messaging.Msg

	OutPort messaging.Port
}

func newAgent(engine timing.EventScheduler, freq timing.Freq, name string, outPort messaging.Port) *agent {
	a := new(agent)
	a.TickingComponent = modeling.NewTickingComponent(name, engine, freq, a)
	a.OutPort = outPort
	a.OutPort.SetComponent(a)

	return a
}

func (a *agent) Tick() bool {
	madeProgress := false

	msgIn := a.OutPort.RetrieveIncoming()
	if msgIn != nil {
		a.msgsIn = append(a.msgsIn, msgIn)
		madeProgress = true
	}

	if len(a.msgsOut) > 0 && a.OutPort.CanSend() {
		a.OutPort.Send(a.msgsOut[0])
		madeProgress = true
		a.msgsOut = a.msgsOut[1:]
	}

	return madeProgress
}

var _ = Describe("Direct Connection Integration", func() {
	var (
		mockCtrl        *gomock.Controller
		engine          timing.Engine
		connection      *Comp
		agents          []*agent
		numAgents       = 10
		numMsgsPerAgent = 1000
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()
		connection = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Conn")
		agents = nil
		for i := 0; i < numAgents; i++ {
			a := newAgent(engine, 1*timing.GHz, fmt.Sprintf("Agent[%d]", i),
				messaging.NewPort(nil, 4, 4, fmt.Sprintf("Agent[%d].OutPort", i)))
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
				msg := newTestMsg()
				msg.Src = agent.OutPort.AsRemote()
				msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				for msg.Dst == msg.Src {
					msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				}
				msg.ID = timing.GetIDGenerator().Generate()
				agent.msgsOut = append(agent.msgsOut, msg)
			}
			agent.TickLater()
		}

		engine.Run()

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
	connection := MakeBuilder().
		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
		Build("Conn")
	agents := make([]*agent, 0, numAgents)

	for i := 0; i < numAgents; i++ {
		a := newAgent(engine, 1*timing.GHz, fmt.Sprintf("Agent%d", i),
			messaging.NewPort(nil, 4, 4, fmt.Sprintf("Agent%d.OutPort", i)))
		agents = append(agents, a)
		connection.PlugIn(a.OutPort)
	}

	for _, agent := range agents {
		for i := 0; i < numMsgsPerAgent; i++ {
			msg := newTestMsg()
			msg.Src = agent.OutPort.AsRemote()
			msg.Dst = agents[r.Intn(len(agents))].OutPort.AsRemote()

			for msg.Dst == msg.Src {
				msg.Dst = agents[r.Intn(len(agents))].OutPort.AsRemote()
			}

			msg.ID = timing.GetIDGenerator().Generate()

			agent.msgsOut = append(agent.msgsOut, msg)
		}

		agent.TickLater()
	}

	engine.Run()

	return engine.CurrentTime()
}
