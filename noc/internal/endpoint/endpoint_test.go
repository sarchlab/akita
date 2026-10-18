package endpoint

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	gomock "go.uber.org/mock/gomock"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestEndPoint(t *testing.T) {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		sim               timing.Simulation
		devicePort        *twowaybuffered.Port
		networkPort       *MockPort
		defaultSwitchPort *MockPort
		endPoint          *Comp
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		mockCtrl = gomock.NewController(t)
		engine = NewMockEngine(mockCtrl)
		engine.EXPECT().RegisterHandler(gomock.Any(), gomock.Any()).AnyTimes()
		engine.EXPECT().CurrentTime().Return(timing.VTimeInPicoSec(0)).AnyTimes()
		sim = testsim.New(engine, false)
		devicePort = twowaybuffered.NewPort(1, 1)
		devicePort.BindOwner(payloadOwner{"Device"}, "DevicePort")
		engine.EXPECT().Schedule(gomock.Any()).AnyTimes()
		networkPort = NewMockPort(mockCtrl)
		networkPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("NetworkPort")).
			AnyTimes()
		defaultSwitchPort = NewMockPort(mockCtrl)
		defaultSwitchPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("DefaultSwitchPort")).
			AnyTimes()

		networkPort.EXPECT().Name().Return("EndPoint.NetworkPort").AnyTimes()
		networkPort.EXPECT().Owner().Return(nil)
		networkPort.EXPECT().BindOwner(gomock.Any(), "EndPoint.NetworkPort")

		spec := DefaultSpec()

		spec.Freq = 1
		spec.FlitByteSize = 32

		endPoint = MakeBuilder().
			WithSimulation(sim).
			WithSpec(spec).
			WithPeer(defaultSwitchPort).
			Build("EndPoint")

		endPoint.BindPort("NetworkPort", networkPort)
		networkPort.EXPECT().Owner().Return(endPoint).AnyTimes()
		link := &testLink{ports: []messaging.Port{networkPort, defaultSwitchPort}}
		networkPort.EXPECT().Connection().Return(link).AnyTimes()
		defaultSwitchPort.EXPECT().Connection().Return(link).AnyTimes()
		defaultSwitchPort.EXPECT().Owner().Return(payloadOwner{"Peer"}).AnyTimes()
		DeviceConnection(endPoint).BindPort(devicePort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	t.Run("should send flits", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:           sim.NewID(),
			Src:          devicePort.AsRemote(),
			Dst:          "OtherDevice",
			TrafficBytes: 33,
			Payload:      memprotocol.WriteReq{Address: 64, Data: []byte{1, 2, 3}},
		}

		networkPort.EXPECT().PeekIncoming().Return(messaging.Msg{}, false).AnyTimes()

		devicePort.Send(msg)

		madeProgress := modelingtest.Tick(endPoint)
		require.True(t, madeProgress)

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(sent messaging.Msg) {
			flit := sent
			require.Equal(t, networkPort.AsRemote(), flit.Src)
			require.Equal(t, defaultSwitchPort.AsRemote(), flit.Dst)
			require.Equal(t, 0, flit.Payload.(packetization.Flit).SeqID)
			require.Equal(t, 2, flit.Payload.(packetization.Flit).NumFlitInMsg)
			require.Equal(t, msg, flit.Payload.(packetization.Flit).Msg)
			require.Equal(t, msg.ID, flit.Payload.(packetization.Flit).MsgID)
		})

		madeProgress = modelingtest.Tick(endPoint)
		require.True(t, madeProgress)

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(sent messaging.Msg) {
			flit := sent
			require.Equal(t, networkPort.AsRemote(), flit.Src)
			require.Equal(t, defaultSwitchPort.AsRemote(), flit.Dst)
			require.Equal(t, 1, flit.Payload.(packetization.Flit).SeqID)
			require.Equal(t, 2, flit.Payload.(packetization.Flit).NumFlitInMsg)
			require.Nil(t, flit.Payload.(packetization.Flit).Msg.Payload)
			require.Equal(t, msg.ID, flit.Payload.(packetization.Flit).MsgID)
		})

		madeProgress = modelingtest.Tick(endPoint)

		require.True(t, madeProgress)

		madeProgress = modelingtest.Tick(endPoint)

		require.False(t, madeProgress)
	})

	t.Run("should receive message", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Dst: devicePort.AsRemote(),
		}

		flit0 := messaging.Msg{Payload: packetization.Flit{
			SeqID:        0,
			NumFlitInMsg: 2,
			MsgID:        msg.ID, Dst: msg.Dst, Msg: msg},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		flit1 := messaging.Msg{Payload: packetization.Flit{
			SeqID:        1,
			NumFlitInMsg: 2,
			MsgID:        msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		networkPort.EXPECT().PeekIncoming().Return(flit0, true)
		networkPort.EXPECT().PeekIncoming().Return(flit1, true)
		networkPort.EXPECT().PeekIncoming().Return(messaging.Msg{}, false).Times(3)
		networkPort.EXPECT().RetrieveIncoming().Times(2)

		madeProgress := modelingtest.Tick(endPoint)
		require.True(t, madeProgress)

		madeProgress = modelingtest.Tick(endPoint)
		require.True(t, madeProgress)

		madeProgress = modelingtest.Tick(endPoint)
		require.True(t, madeProgress)

		madeProgress = modelingtest.Tick(endPoint)
		require.True(t, madeProgress)

		madeProgress = modelingtest.Tick(endPoint)
		require.False(t, madeProgress)
		received, ok := devicePort.PeekIncoming()
		require.True(t, ok)
		require.Equal(t, msg, received)
	})
}
