package direct

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/sarchlab/akita/v5/internal/testsim"
	akitasim "github.com/sarchlab/akita/v5/sim"

	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

type testMsg struct {
}

func newTestMsg(sim timing.Simulation) messaging.Msg {
	return messaging.Msg{ID: sim.NewID(), Payload: testMsg{}}
}

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestDirectConnection(t *testing.T) {
	var (
		mockCtrl   *gomock.Controller
		port1      *MockPort
		port2      *MockPort
		engine     *MockEngine
		sim        timing.Simulation
		connection *Connection
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		mockCtrl = gomock.NewController(t)

		port1 = NewMockPort(mockCtrl)
		port1.EXPECT().AsRemote().Return(messaging.RemotePort("port1")).AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().AsRemote().Return(messaging.RemotePort("port2")).AnyTimes()

		engine = NewMockEngine(mockCtrl)
		engine.EXPECT().RegisterHandler(gomock.Any(), gomock.Any()).AnyTimes()
		sim = testsim.New(engine, false)
		connection = NewConnection("Direct", sim, timing.GHz)

		port1.EXPECT().Owner().Return(&agent{}).AnyTimes()
		port1.EXPECT().Connection().Return(nil).AnyTimes()
		port1.EXPECT().BindConnection(connection)
		connection.BindPort(port1)

		port2.EXPECT().Owner().Return(&agent{}).AnyTimes()
		port2.EXPECT().Connection().Return(nil).AnyTimes()
		port2.EXPECT().BindConnection(connection)
		connection.BindPort(port2)
	}

	t.Run("should forward when handling tick event", func(t *testing.T) {
		setup1(t)

		engine.EXPECT().CurrentTime().Return(timing.VTimeInPicoSec(10000))

		tick := ticking.MakeEvent(sim.NewID(), timing.VTimeInPicoSec(10000), connection.Name())

		msg1 := newTestMsg(sim)
		msg1.Src = port1.AsRemote()
		msg1.Dst = port2.AsRemote()

		msg2 := newTestMsg(sim)
		msg2.Src = port2.AsRemote()
		msg2.Dst = port1.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg1, true)
		port1.EXPECT().PeekOutgoing().Return(messaging.Msg{}, false)
		port1.EXPECT().RetrieveOutgoing().Return(msg1, true)
		port1.EXPECT().CanDeliver().Return(true)
		port1.EXPECT().Deliver(msg2)

		port2.EXPECT().PeekOutgoing().Return(msg2, true)
		port2.EXPECT().PeekOutgoing().Return(messaging.Msg{}, false)
		port2.EXPECT().RetrieveOutgoing().Return(msg2, true)
		port2.EXPECT().CanDeliver().Return(true)
		port2.EXPECT().Deliver(msg1)

		engine.EXPECT().
			Schedule(gomock.Any()).
			Do(func(evt ticking.Event) {
				require.Equal(t, timing.VTimeInPicoSec(11000), evt.Time())
				require.True(t, evt.IsSecondary())
			})

		connection.Handle(tick)
	})

	t.Run("should keep outgoing messages queued when delivery is blocked", func(t *testing.T) {
		setup1(t)

		tick := ticking.MakeEvent(sim.NewID(), timing.VTimeInPicoSec(10000), connection.Name())

		msg := newTestMsg(sim)
		msg.Src = port1.AsRemote()
		msg.Dst = port2.AsRemote()

		port1.EXPECT().PeekOutgoing().Return(msg, true)
		port2.EXPECT().CanDeliver().Return(false)
		port2.EXPECT().PeekOutgoing().Return(messaging.Msg{}, false)

		connection.Handle(tick)
	})
}

// agent is a test double that sends its messages out of OutPort and records
// what it receives, ticking while it makes progress.
type agent struct {
	*ticking.Scheduler
	hooking.HookableBase

	name    string
	msgsOut []messaging.Msg
	msgsIn  []messaging.Msg

	OutPort messaging.Port
}

func newAgent(sim timing.Simulation, freq timing.Freq, name string, outPort messaging.Port) *agent {
	a := &agent{
		Scheduler: ticking.NewScheduler(name, sim, freq),
		name:      name,
		OutPort:   outPort,
	}
	a.OutPort.BindOwner(a, name+".Out")
	sim.Engine().RegisterHandler(name, a)

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

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestDirectConnectionIntegration(t *testing.T) {
	var (
		engine          timing.Engine
		sim             timing.Simulation
		connection      *Connection
		agents          []*agent
		numAgents       = 10
		numMsgsPerAgent = 1000
	)

	setup3 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		connection = NewConnection("Conn", sim, timing.GHz)
		agents = nil
		for i := 0; i < numAgents; i++ {
			a := newAgent(sim, 1*timing.GHz, fmt.Sprintf("Agent[%d]", i),
				twowaybuffered.NewPort(4, 4))
			agents = append(agents, a)
			connection.BindPort(a.OutPort)
		}
	}

	t.Run("should deliver all messages", func(t *testing.T) {
		setup3(t)

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

		require.NoError(t, sim.Initialize())
		require.NoError(t, engine.Run())

		totalRecvedMsgCount := 0
		for _, agent := range agents {
			totalRecvedMsgCount += len(agent.msgsIn)
		}

		require.Equal(t, numAgents*numMsgsPerAgent, totalRecvedMsgCount)
	})

	t.Run("should run deterministicly", func(t *testing.T) {
		setup3(t)

		seed := time.Now().UTC().UnixNano()
		time1 := directConnectionTest(t, seed)
		time2 := directConnectionTest(t, seed)

		require.Equal(t, time2, time1)
	})
}

func directConnectionTest(t *testing.T, seed int64) timing.VTimeInPicoSec {
	t.Helper()

	r := rand.New(rand.NewSource(seed))

	numAgents := 100
	numMsgsPerAgent := 1000
	sim := akitasim.MakeBuilder().Build()
	engine := sim.Engine().(*timing.SerialEngine)
	connection := NewConnection("Conn", sim, timing.GHz)
	agents := make([]*agent, 0, numAgents)

	for i := 0; i < numAgents; i++ {
		a := newAgent(sim, 1*timing.GHz, fmt.Sprintf("Agent%d", i),
			twowaybuffered.NewPort(4, 4))
		agents = append(agents, a)
		connection.BindPort(a.OutPort)
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

	require.NoError(t, sim.Initialize())
	require.NoError(t, engine.Run())

	return engine.CurrentTime()
}

var _ = messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{testMsg{}}})

func (a *agent) BindPort(slot string, p messaging.Port) { p.BindOwner(a, a.Name()+"."+slot) }
