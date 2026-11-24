package switches

import (
	"encoding/json"
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

func TestSwitch(t *testing.T) {
	var (
		mockCtrl     *gomock.Controller
		engine       *MockEngine
		sim          *testsim.Simulation
		port1, port2 *MockPort
		dstPort      *MockPort
		routingTable *MockRoutingTable
		sw           *Comp
		rfsMW        *routeForwardSendMW
		rpMW         *receivePipelineMW
		now          timing.VTimeInPicoSec
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		now = 0
		mockCtrl = gomock.NewController(t)
		engine = NewMockEngine(mockCtrl)
		engine.EXPECT().RegisterHandler(gomock.Any(), gomock.Any()).AnyTimes()
		engine.EXPECT().CurrentTime().DoAndReturn(func() timing.VTimeInPicoSec { return now }).AnyTimes()
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
			return Link{Remote: remote.AsRemote(),
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
			ID:  sim.NewID(),
			Dst: port1.AsRemote(),
		}

		port1.EXPECT().PeekIncoming().Return(flit, true)
		port1.EXPECT().RetrieveIncoming()
		port2.EXPECT().PeekIncoming().Return(messaging.Msg{}, false)

		routingTable.EXPECT().FindPort(dstPort.AsRemote()).Return(port2.AsRemote())
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
			ID:  sim.NewID(),
			Dst: port1.AsRemote(),
		}

		// Fill pipeline so it can't accept
		next := &sw.State
		next.PortComplexes[0].Pipeline.Push(routedFlit{TaskID: 1})

		port1.EXPECT().PeekIncoming().Return(flit, true)
		port2.EXPECT().PeekIncoming().Return(messaging.Msg{}, false)

		madeProgress := rpMW.startProcessing()

		require.False(t, madeProgress)
	})

	t.Run("should tick the pipelines", func(t *testing.T) {
		setup1(t)

		// Place an item in pipeline stage 0 for port1
		next := &sw.State
		next.PortComplexes[0].Pipeline.Push(routedFlit{
			Flit:   messaging.Msg{ID: 100, Payload: packetization.Flit{}},
			TaskID: 101,
		})

		now = sw.Spec.Freq.Period()
		madeProgress := rpMW.movePipeline()

		require.True(t, madeProgress)
		// For latency=1, the item should have moved to Waiting
		next = &sw.State
		require.Len(t, next.PortComplexes[0].Pipeline.Stages(), 0)
		require.Equal(t, 1, next.PortComplexes[0].Waiting.Size())
	})

	t.Run("blocked output shares waiting slots and preserves each output FIFO", func(t *testing.T) {
		setup1(t)
		a, b := &sw.State.PortComplexes[0], &sw.State.PortComplexes[1]
		a.NumInputChannel = 2
		a.Waiting = queueing.MakeBuffer[routedFlit](4)
		for i, dst := range []int{0, 1, 0, 1} {
			a.Waiting.Push(routedFlit{TaskID: uint64(i + 1), OutputBufIdx: dst})
		}
		a.SendOutBuffer.Push(routedFlit{})
		require.False(t, a.Waiting.CanPush())
		require.False(t, rfsMW.forward(), "new arrivals need one routing cycle")
		require.True(t, rfsMW.route())
		for _, id := range []uint64{2, 4} {
			require.True(t, rfsMW.forward())
			item, ok := b.SendOutBuffer.Pop()
			require.True(t, ok)
			require.Equal(t, id, item.TaskID)
			// A single blocked output can claim every slot freed by B.
			a.Waiting.Push(routedFlit{TaskID: id + 4, OutputBufIdx: 0})
			require.True(t, rfsMW.route())
		}
		require.False(t, a.Waiting.CanPush())
		require.False(t, rfsMW.forward())
		a.SendOutBuffer.Pop()
		for _, id := range []uint64{1, 3, 6, 8} {
			require.True(t, rfsMW.forward())
			item, ok := a.SendOutBuffer.Pop()
			require.True(t, ok)
			require.Equal(t, id, item.TaskID)
		}
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
			ID: sim.NewID(),
		}

		// Place flit in forward buffer of port1, targeting sendOutBuffer of port2
		next := &sw.State
		next.PortComplexes[0].Waiting =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].Waiting.Push(routedFlit{Flit: flit, OutputBufIdx: 1})

		require.True(t, rfsMW.route())
		madeProgress := rfsMW.forward()

		require.True(t, madeProgress)
		next = &sw.State
		require.Equal(t, 0, next.PortComplexes[0].Waiting.Size())
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
			ID: sim.NewID(),
		}

		// Fill sendOut buffer to capacity, forward buffer targets port2
		next := &sw.State
		next.PortComplexes[0].Waiting =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[0].Waiting.Push(routedFlit{Flit: flit, OutputBufIdx: 1})
		next.PortComplexes[1].SendOutBuffer =
			queueing.MakeBuffer[routedFlit](1)
		next.PortComplexes[1].SendOutBuffer.Push(routedFlit{Flit: messaging.Msg{ID: 400, Payload: packetization.Flit{}}})

		require.True(t, rfsMW.route())
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
			ID: sim.NewID(),
		}

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
			ID: sim.NewID(),
		}

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

func TestWaitingCheckpointBounds(t *testing.T) {
	for _, data := range []string{
		`{"num_input_channel":1,"route_buffer":{"cap":1},"forward_buffer":{"cap":1}}`,
		`{"num_input_channel":1,"waiting":{"cap":2},"ready":1}`,
		`{"num_input_channel":1,"waiting":{"cap":2},"ready":-1}`,
	} {
		var port portComplexState
		require.ErrorContains(t, json.Unmarshal([]byte(data), &port), "invalid waiting buffer")
	}
}
