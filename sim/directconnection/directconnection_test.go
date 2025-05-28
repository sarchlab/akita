package directconnection

import (
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	gomock "go.uber.org/mock/gomock"
)

type sampleMsg struct {
	sim.MsgMeta
}

func NewSampleMsg() *sampleMsg {
	m := &sampleMsg{}
	return m
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *sampleMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
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
		port1.EXPECT().AsRemote().Return(sim.RemotePort("port1")).AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().AsRemote().Return(sim.RemotePort("port2")).AnyTimes()

		engine = NewMockEngine(mockCtrl)
		connection = MakeBuilder().
			WithEngine(engine).
			WithFreq(1).
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
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		tick := sim.MakeTickEvent(connection, sim.VTimeInSec(10))

		msg1 := NewSampleMsg()
		msg1.Src = port1.AsRemote()
		msg1.Dst = port2.AsRemote()

		msg2 := NewSampleMsg()
		msg2.Src = port2.AsRemote()
		msg2.Dst = port1.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg1)
		port1.EXPECT().PeekOutgoing().Return(nil)
		port1.EXPECT().RetrieveOutgoing().Return(msg1)
		port1.EXPECT().Deliver(msg2).Return(nil)

		port2.EXPECT().PeekOutgoing().Return(msg2)
		port2.EXPECT().PeekOutgoing().Return(nil)
		port2.EXPECT().RetrieveOutgoing().Return(msg2)
		port2.EXPECT().Deliver(msg1).Return(nil)

		engine.EXPECT().
			Schedule(gomock.Any()).
			Do(func(evt sim.TickEvent) {
				Expect(evt.Time()).To(Equal(sim.VTimeInSec(11)))
				Expect(evt.IsSecondary()).To(BeTrue())
			})

		connection.Handle(tick)
	})
})

type agent struct {
	*sim.TickingComponent

	msgsOut []sim.Msg
	msgsIn  []sim.Msg

	OutPort sim.Port
}

func newAgent(engine sim.Engine, freq sim.Freq, name string) *agent {
	a := new(agent)
	a.TickingComponent = sim.NewTickingComponent(name, engine, freq, a)
	a.OutPort = sim.NewPort(a, 4, 4, name+".OutPort")

	return a
}

func (a *agent) Tick() bool {
	madeProgress := false

	msgIn := a.OutPort.RetrieveIncoming()
	if msgIn != nil {
		a.msgsIn = append(a.msgsIn, msgIn)
		madeProgress = true
	}

	if len(a.msgsOut) > 0 {
		err := a.OutPort.Send(a.msgsOut[0])
		if err == nil {
			madeProgress = true
			a.msgsOut = a.msgsOut[1:]
		}
	}

	return madeProgress
}

var _ = Describe("Direct Connection Integration", func() {
	var (
		mockCtrl        *gomock.Controller
		engine          sim.Engine
		connection      *Comp
		agents          []*agent
		numAgents       = 10
		numMsgsPerAgent = 1000
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()
		connection = MakeBuilder().WithEngine(engine).WithFreq(1).Build("Conn")
		agents = nil
		for i := 0; i < numAgents; i++ {
			a := newAgent(engine, 1, fmt.Sprintf("Agent[%d]", i))
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
				msg := NewSampleMsg()
				msg.Src = agent.OutPort.AsRemote()
				msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				for msg.Dst == msg.Src {
					msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				}
				msg.ID = fmt.Sprintf("%s(%d)->%s",
					agent.Name(), i, msg.Dst)
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

func directConnectionTest(seed int64) sim.VTimeInSec {
	r := rand.New(rand.NewSource(seed))

	numAgents := 100
	numMsgsPerAgent := 1000
	engine := sim.NewSerialEngine()
	connection := MakeBuilder().WithEngine(engine).WithFreq(1).Build("Conn")
	agents := make([]*agent, 0, numAgents)

	for i := 0; i < numAgents; i++ {
		a := newAgent(engine, 1, fmt.Sprintf("Agent%d", i))
		agents = append(agents, a)
		connection.PlugIn(a.OutPort)
	}

	for _, agent := range agents {
		for i := 0; i < numMsgsPerAgent; i++ {
			msg := NewSampleMsg()
			msg.Src = agent.OutPort.AsRemote()
			msg.Dst = agents[r.Intn(len(agents))].OutPort.AsRemote()

			for msg.Dst == msg.Src {
				msg.Dst = agents[r.Intn(len(agents))].OutPort.AsRemote()
			}

			msg.ID = fmt.Sprintf("%s(%d)->%s",
				agent.Name(), i, msg.Dst)

			agent.msgsOut = append(agent.msgsOut, msg)
		}

		agent.TickLater()
	}

	engine.Run()

	return engine.CurrentTime()
}
