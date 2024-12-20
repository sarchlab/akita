package directconnection

import (
	"fmt"
	"math/rand"
	"time"

	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type sampleMsg struct {
	modeling.MsgMeta
}

func (m sampleMsg) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

func (m sampleMsg) ID() string {
	return m.MsgMeta.ID
}

func (m sampleMsg) Serialize() (map[string]any, error) {
	return map[string]any{
		"id":            m.ID(),
		"src":           m.Src,
		"dst":           m.Dst,
		"traffic_class": m.TrafficClass,
		"traffic_bytes": m.TrafficBytes,
	}, nil
}

func (m sampleMsg) Deserialize(
	data map[string]any,
) error {
	m.MsgMeta.ID = data["id"].(string)
	m.MsgMeta.Src = data["src"].(modeling.RemotePort)
	m.MsgMeta.Dst = data["dst"].(modeling.RemotePort)
	m.MsgMeta.TrafficClass = data["traffic_class"].(int)
	m.MsgMeta.TrafficBytes = data["traffic_bytes"].(int)

	return nil
}

func (m sampleMsg) Clone() modeling.Msg {
	cloneMsg := m
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
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
		port1.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("port1")).
			AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("port2")).
			AnyTimes()

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
		engine.EXPECT().Now().Return(timing.VTimeInSec(10))

		tick := timing.MakeTickEvent(connection, timing.VTimeInSec(10))

		msg1 := sampleMsg{}
		msg1.Src = port1.AsRemote()
		msg1.Dst = port2.AsRemote()

		msg2 := sampleMsg{}
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
			Do(func(evt timing.TickEvent) {
				Expect(evt.Time()).To(Equal(timing.VTimeInSec(11)))
				Expect(evt.IsSecondary()).To(BeTrue())
			})

		connection.Handle(tick)
	})
})

type agent struct {
	*modeling.TickingComponent

	msgsOut []modeling.Msg
	msgsIn  []modeling.Msg

	OutPort modeling.Port
}

func (a *agent) ID() string {
	return a.Name()
}

func (a *agent) Serialize() (map[string]any, error) {
	panic("not implemented")
}

func (a *agent) Deserialize(data map[string]any) error {
	panic("not implemented")
}

func newAgent(engine timing.Engine, freq timing.Freq, name string) *agent {
	a := new(agent)
	a.TickingComponent = modeling.NewTickingComponent(name, engine, freq, a)
	a.OutPort = modeling.NewPort(a, 4, 4, name+".OutPort")

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
		engine          timing.Engine
		connection      *Comp
		agents          []*agent
		numAgents       = 10
		numMsgsPerAgent = 1000
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()
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
				msg := sampleMsg{}
				msg.Src = agent.OutPort.AsRemote()
				msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()

				for msg.Dst == msg.Src {
					msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
				}

				msg.MsgMeta.ID = fmt.Sprintf("%s(%d)->%s",
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

	It("should run deterministically", func() {
		seed := time.Now().UTC().UnixNano()
		time1 := directConnectionTest(seed)
		time2 := directConnectionTest(seed)

		Expect(time1).To(Equal(time2))
	})
})

func directConnectionTest(seed int64) timing.VTimeInSec {
	rand.Seed(seed)

	numAgents := 100
	numMsgsPerAgent := 1000
	engine := timing.NewSerialEngine()
	connection := MakeBuilder().WithEngine(engine).WithFreq(1).Build("Conn")
	agents := make([]*agent, 0, numAgents)

	for i := 0; i < numAgents; i++ {
		a := newAgent(engine, 1, fmt.Sprintf("Agent%d", i))
		agents = append(agents, a)
		connection.PlugIn(a.OutPort)
	}

	for _, agent := range agents {
		for i := 0; i < numMsgsPerAgent; i++ {
			msg := sampleMsg{}
			msg.Src = agent.OutPort.AsRemote()
			msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()

			for msg.Dst == msg.Src {
				msg.Dst = agents[rand.Intn(len(agents))].OutPort.AsRemote()
			}

			msg.MsgMeta.ID = fmt.Sprintf("%s(%d)->%s",
				agent.Name(), i, msg.Dst)

			agent.msgsOut = append(agent.msgsOut, msg)
		}

		agent.TickLater()
	}

	engine.Run()

	return engine.Now()
}
