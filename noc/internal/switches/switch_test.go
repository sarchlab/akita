package switches

import (
	"fmt"

	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestSwitch(t *testing.T) {
	var (
		mockCtrl     *gomock.Controller
		engine       *MockEngine
		sim          timing.Simulation
		port1, port2 *MockPort
		dstPort      *MockPort
		routingTable *MockRoutingTable
		sw           *Comp
		rfsMW        *routeForwardSendMW
		rpMW         *receivePipelineMW
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		mockCtrl = gomock.NewController(t)
		engine = NewMockEngine(mockCtrl)
		engine.EXPECT().RegisterHandler(gomock.Any(), gomock.Any()).AnyTimes()
		sim = testsim.New(engine, false)
		port1 = NewMockPort(mockCtrl)
		port1.EXPECT().AsRemote().
			Return(messaging.RemotePort("LocalPort1")).
			AnyTimes()
		port1.EXPECT().Name().
			Return("Switch.Links[0]").
			AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().AsRemote().
			Return(messaging.RemotePort("LocalPort2")).
			AnyTimes()
		port2.EXPECT().Name().
			Return("Switch.Links[1]").
			AnyTimes()

		remote1 := NewMockPort(mockCtrl)
		remote1.EXPECT().AsRemote().
			Return(messaging.RemotePort("RemotePort1")).
			AnyTimes()

		remote2 := NewMockPort(mockCtrl)
		remote2.EXPECT().AsRemote().
			Return(messaging.RemotePort("RemotePort2")).
			AnyTimes()

		dstPort = NewMockPort(mockCtrl)
		dstPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("DstPort")).
			AnyTimes()

		routingTable = NewMockRoutingTable(mockCtrl)

		spec := DefaultSpec()
		spec.Freq = 1

		for _, p := range []*MockPort{port1, port2} {
			p.EXPECT().Owner().Return(nil)
			p.EXPECT().BindOwner(gomock.Any(), gomock.Any())
		}

		link := func(remote *MockPort) Link {
			return Link{
				Remote:           remote.AsRemote(),
				NumInputChannel:  1,
				NumOutputChannel: 1,
				Latency:          1,
			}
		}

		wiring := &testWiring{links: []Link{link(remote1), link(remote2)},
			table: routingTable}
		sw = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).WithResources(Resources{Wiring: wiring}).
			Build("Switch")
		swPorts := Ports{Links: []messaging.Port{port1, port2}}
		for i, p := range swPorts.Links {
			sw.BindPort(fmt.Sprintf("Links[%d]", i), p)
			p.(*MockPort).EXPECT().Owner().Return(sw).AnyTimes()
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		rfsMW = sw.Middlewares.RouteForwardSend
		rfsMW.routingTable = routingTable
		rpMW = sw.Middlewares.ReceivePipeline
	}

	t.Run("should start processing", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			Dst:          port1.AsRemote(),
			TrafficClass: "packetization.Flit"}

		port1.EXPECT().PeekIncoming().Return(flit, true)
		port1.EXPECT().RetrieveIncoming()
		port2.EXPECT().PeekIncoming().Return(messaging.Msg{}, false)

		madeProgress := rpMW.startProcessing()

		require.True(t, madeProgress)
		// Verify flit was accepted into pipeline
		next := &sw.State
		require.Len(t, next.PortComplexes[0].Pipeline.Stages(), 1)
		require.Equal(t, flit.ID, next.PortComplexes[0].Pipeline.Stages()[0].Item.Flit.ID)
	})

	t.Run("should not start processing if pipeline is busy", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			Dst:          port1.AsRemote(),
			TrafficClass: "packetization.Flit"}

		// Fill pipeline so it can't accept
		next := &sw.State
		next.PortComplexes[0].Pipeline.Accept(routedFlit{TaskID: 1})

		port1.EXPECT().PeekIncoming().Return(flit, true)
		port2.EXPECT().PeekIncoming().Return(messaging.Msg{}, false)

		madeProgress := rpMW.startProcessing()

		require.False(t, madeProgress)
	})

	t.Run("should tick the pipelines", func(t *testing.T) {
		setup1(t)

		// Place an item in pipeline stage 0 for port1
		next := &sw.State
		next.PortComplexes[0].Pipeline.Accept(routedFlit{
			Flit:   messaging.Msg{ID: 100, Payload: packetization.Flit{}},
			TaskID: 101,
		})

		madeProgress := rpMW.movePipeline()

		require.True(t, madeProgress)
		// For latency=1, the item should have moved to RouteBuffer
		next = &sw.State
		require.Len(t, next.PortComplexes[0].Pipeline.Stages(), 0)
		require.Equal(t, 1, next.PortComplexes[0].RouteBuffer.Size())
	})

	t.Run("should route", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		// Place item in route buffer for port1
		next := &sw.State
		next.PortComplexes[0].RouteBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].RouteBuffer.Push(routedFlit{Flit: flit, TaskID: 200, RouteTo: dstPort.AsRemote()})

		routingTable.EXPECT().
			FindPort(dstPort.AsRemote()).
			Return(port2.AsRemote())

		madeProgress := rfsMW.route()

		require.True(t, madeProgress)
		next = &sw.State
		require.Equal(t, 0, next.PortComplexes[0].RouteBuffer.Size())
		require.Equal(t, 1, next.PortComplexes[0].ForwardBuffer.Size())
	})

	t.Run("should not route if forward buffer is full", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		// Place item in route buffer and fill forward buffer
		next := &sw.State
		next.PortComplexes[0].RouteBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].RouteBuffer.Push(routedFlit{Flit: flit, TaskID: 200, RouteTo: dstPort.AsRemote()})
		next.PortComplexes[0].ForwardBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].ForwardBuffer.Push(routedFlit{Flit: messaging.Msg{ID: 300, Payload: packetization.Flit{}}})

		madeProgress := rfsMW.route()

		require.False(t, madeProgress)
	})

	t.Run("should forward", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		// Place flit in forward buffer of port1, targeting sendOutBuffer of port2
		next := &sw.State
		next.PortComplexes[0].ForwardBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].ForwardBuffer.Push(routedFlit{Flit: flit, OutputBufIdx: 1})

		madeProgress := rfsMW.forward()

		require.True(t, madeProgress)
		next = &sw.State
		require.Equal(t, 0, next.PortComplexes[0].ForwardBuffer.Size())
		require.Equal(t, 1, next.PortComplexes[1].SendOutBuffer.Size())
	})

	t.Run("should not forward if the output buffer is busy", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		// Fill sendOut buffer to capacity, forward buffer targets port2
		next := &sw.State
		next.PortComplexes[0].ForwardBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].ForwardBuffer.Push(routedFlit{Flit: flit, OutputBufIdx: 1})
		next.PortComplexes[1].SendOutBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[1].SendOutBuffer.Push(routedFlit{Flit: messaging.Msg{ID: 400, Payload: packetization.Flit{}}})

		madeProgress := rfsMW.forward()

		require.False(t, madeProgress)
	})

	t.Run("should send flits out", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		// Place flit in sendOutBuffer of port2
		next := &sw.State
		next.PortComplexes[1].SendOutBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[1].SendOutBuffer.Push(routedFlit{Flit: flit})

		port2.EXPECT().CanSend().Return(true)
		port2.EXPECT().Send(gomock.Any())

		madeProgress := rfsMW.sendOut()

		require.True(t, madeProgress)
		next = &sw.State
		require.Equal(t, 0, next.PortComplexes[1].SendOutBuffer.Size())
	})

	t.Run("should wait if port is busy sending flits out", func(t *testing.T) {
		setup1(t)

		msg := messaging.Msg{
			ID:  sim.NewID(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := messaging.Msg{Payload: packetization.Flit{
			MsgID: msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		// Place flit in sendOutBuffer of port2
		next := &sw.State
		next.PortComplexes[1].SendOutBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[1].SendOutBuffer.Push(routedFlit{Flit: flit})

		port2.EXPECT().CanSend().Return(false)

		madeProgress := rfsMW.sendOut()

		require.False(t, madeProgress)
		// Flit should still be in send buffer
		next = &sw.State
		require.Equal(t, 1, next.PortComplexes[1].SendOutBuffer.Size())
	})
}
