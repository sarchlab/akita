package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Because the controller now owns its ports (they are no
// longer injectable), tests feed requests with Deliver and read responses with
// RetrieveOutgoing; the port still needs a connection so its send/retrieve
// notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(_ messaging.Port)      {}

// makePorts creates the ports of the controller named name: a Top port with
// the given buffer size and a Control port with a 16-message buffer.
func makePorts(simulation modeling.Simulation, name string, topBufSize int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: topBufSize, OutgoingCapacity: topBufSize}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build(),
	}
}

func TestIdealMemoryController(t *testing.T) {
	var (
		engine        timing.Engine
		sim           *testsim.Simulation
		storage       *mem.Storage
		memController *Comp
		topPort       messaging.Port
	)

	// build constructs a controller with the given Top-port buffer size, injects
	// the shared storage, and plugs a noopConn so its ports can be driven.
	build := func(topBufSize int) {
		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		spec := DefaultSpec()
		spec.Width = 1
		spec.Latency = 10

		memController = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("MemCtrl")
		memControllerPorts := makePorts(sim, "MemCtrl", topBufSize)
		memController.BindPort("Top", memControllerPorts.Top)
		memController.BindPort("Control", memControllerPorts.Control)

		topPort = memController.Ports.Top
		conn := &noopConn{}
		conn.BindPort(topPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	makeReadReq := func() messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
		}

		return req
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		storage = mem.NewStorage(1 * mem.MB)
		build(16)
	}

	t.Run("should accept read request and add to inflight transactions", func(t *testing.T) {
		setup1(t)

		topPort.Deliver(makeReadReq())

		madeProgress := modelingtest.Tick(memController)

		require.True(t, madeProgress)
		state := memController.State
		require.Len(t, state.InflightTransactions, 1)
		require.True(t, state.InflightTransactions[0].IsRead)
		// After first tick: latency=10, decrement once → 9
		require.Equal(t, 9, state.InflightTransactions[0].CycleLeft)
	})

	t.Run("should accept write request and add to inflight transactions", func(t *testing.T) {
		setup1(t)

		writeReq := messaging.Msg{Payload: memprotocol.WriteReq{
			Address:   0,
			Data:      []byte{0, 1, 2, 3},
			DirtyMask: []bool{false, false, true, false}},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote()}

		writeReq.TrafficBytes = len(writeReq.Payload.(memprotocol.WriteReq).Data) + 12
		topPort.Deliver(writeReq)

		madeProgress := modelingtest.Tick(memController)
		require.True(t, madeProgress)
		state := memController.State
		require.Len(t, state.InflightTransactions, 1)
		require.False(t, state.InflightTransactions[0].IsRead)
		require.Equal(t, 9, state.InflightTransactions[0].CycleLeft)
	})

	t.Run("should send read response after latency ticks", func(t *testing.T) {
		setup1(t)

		topPort.Deliver(makeReadReq())

		// Tick 1: take request, CycleLeft: 10 → 9
		modelingtest.Tick(memController)

		// Ticks 2-9: count down (9 → 2)
		for i := 0; i < 8; i++ {
			modelingtest.Tick(memController)
		}

		state := memController.State
		require.Len(t, state.InflightTransactions, 1)
		require.Equal(t, 1, state.InflightTransactions[0].CycleLeft)

		// Tick 10: CycleLeft 1→0, then send response
		modelingtest.Tick(memController)

		state = memController.State
		require.Len(t, state.InflightTransactions, 0)

		rsp, _ := topPort.RetrieveOutgoing()
		require.IsType(t, messaging.Msg{Payload: memprotocol.DataReadyRsp{}}, rsp)
	})

	t.Run("should send write response after latency ticks", func(t *testing.T) {
		setup1(t)

		writeReq := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0,
			Data:    []byte{0, 1, 2, 3}},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote()}

		writeReq.TrafficBytes = len(writeReq.Payload.(memprotocol.WriteReq).Data) + 12
		topPort.Deliver(writeReq)

		// Tick 1: take request, CycleLeft: 10 → 9
		modelingtest.Tick(memController)

		// Ticks 2-9: count down
		for i := 0; i < 8; i++ {
			modelingtest.Tick(memController)
		}

		// Tick 10: CycleLeft 1→0, send response
		modelingtest.Tick(memController)

		state := memController.State
		require.Len(t, state.InflightTransactions, 0)

		rsp, _ := topPort.RetrieveOutgoing()
		require.IsType(t, messaging.Msg{Payload: memprotocol.WriteDoneRsp{}}, rsp)

		// Verify data was written to storage
		data := storage.Read(0, 4)
		require.Equal(t, []byte{0, 1, 2, 3}, data)
	})

	t.Run("should retry send when port is busy", func(t *testing.T) {
		setup1(t)

		// Rebuild with a single-slot Top port so the outgoing buffer can be
		// forced full.
		build(1)

		// Pre-fill the outgoing buffer so the controller's response Send fails.
		dummy := messaging.Msg{ID: sim.NewID(), Payload: memprotocol.WriteDoneRsp{},
			Src: topPort.AsRemote(),
			Dst: messaging.RemotePort("Agent"),
		}

		topPort.Send(dummy)

		topPort.Deliver(makeReadReq())

		// Tick 1: take request, CycleLeft: 10 → 9
		modelingtest.Tick(memController)

		// Ticks 2-9: count down (8 ticks, 9→1)
		for i := 0; i < 8; i++ {
			modelingtest.Tick(memController)
		}

		state := memController.State
		require.Len(t, state.InflightTransactions, 1)
		require.Equal(t, 1, state.InflightTransactions[0].CycleLeft)

		// Tick 10: CycleLeft 1→0, send fails (outgoing buffer full)
		modelingtest.Tick(memController)

		state = memController.State
		require.Len(t, state.InflightTransactions, 1)
		require.Equal(t, 0, state.InflightTransactions[0].CycleLeft)

		// Free the outgoing buffer, then retry succeeds.
		value0, present0 := topPort.RetrieveOutgoing()
		require.True(t, present0)
		require.Equal(t, dummy, value0)
		modelingtest.Tick(memController)

		state = memController.State
		require.Len(t, state.InflightTransactions, 0)
	})

	t.Run("should write with dirty mask", func(t *testing.T) {
		setup1(t)

		// Pre-write data
		storage.Write(0, []byte{10, 20, 30, 40})

		writeReq := messaging.Msg{Payload: memprotocol.WriteReq{
			Address:   0,
			Data:      []byte{0, 1, 2, 3},
			DirtyMask: []bool{false, false, true, false}},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote()}

		writeReq.TrafficBytes = len(writeReq.Payload.(memprotocol.WriteReq).Data) + 12
		topPort.Deliver(writeReq)

		// Tick 1: take request
		modelingtest.Tick(memController)

		// Ticks 2-9: count down
		for i := 0; i < 8; i++ {
			modelingtest.Tick(memController)
		}

		// Tick 10: send response
		modelingtest.Tick(memController)

		// Check that only dirty bytes were written
		data := storage.Read(0, 4)
		require.Equal(t, []byte{10, 20, 2, 40}, data)
	})

	t.Run("should use Spec for latency and width", func(t *testing.T) {
		setup1(t)

		spec := memController.Spec
		require.Equal(t, 10, spec.Latency)
		require.Equal(t, 1, spec.Width)
	})

	t.Run("should use State for current state", func(t *testing.T) {
		setup1(t)

		state := memController.State
		require.Equal(t, controlStateEnabled, state.ControlState)
	})
}
