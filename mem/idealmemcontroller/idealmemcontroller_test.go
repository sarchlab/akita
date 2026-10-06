package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
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
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

// makePorts creates the ports of the controller named name: a Top port with
// the given buffer size and a Control port with a 16-message buffer.
func makePorts(name string, topBufSize int) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(name+".Top", topBufSize, topBufSize),
		Control: twowaybuffered.NewPort(name+".Control", 16, 16),
	}
}

var _ = Describe("Ideal Memory Controller", func() {
	var (
		engine        timing.Engine
		sim           timing.Simulation
		storage       *mem.Storage
		memController *Comp
		topPort       messaging.Port
	)

	// build constructs a controller with the given Top-port buffer size, injects
	// the shared storage, and plugs a noopConn so its ports can be driven.
	build := func(topBufSize int) {
		spec := Definition.DefaultSpec
		spec.Width = 1
		spec.Latency = 10
		spec.CacheLineSize = 64

		memController = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			WithSpec(spec).
			WithPorts(makePorts("MemCtrl", topBufSize)).
			Build("MemCtrl")

		topPort = memController.Ports.Top
		conn := &noopConn{}
		conn.PlugIn(topPort)
	}

	makeReadReq := func() messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		storage = mem.NewStorage(1 * mem.MB)
		build(16)
	})

	It("should accept read request and add to inflight transactions", func() {
		topPort.Deliver(makeReadReq())

		madeProgress := modelingtest.Tick(memController)

		Expect(madeProgress).To(BeTrue())
		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].IsRead).To(BeTrue())
		// After first tick: latency=10, decrement once → 9
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(9))
	})

	It("should accept write request and add to inflight transactions", func() {
		writeReq := messaging.Msg{Payload: memprotocol.WriteReq{
			Address:   0,
			Data:      []byte{0, 1, 2, 3},
			DirtyMask: []bool{false, false, true, false}},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote()}

		writeReq.TrafficBytes = len(writeReq.Payload.(memprotocol.WriteReq).Data) + 12
		writeReq.TrafficClass = "memprotocol.WriteReq"
		topPort.Deliver(writeReq)

		madeProgress := modelingtest.Tick(memController)
		Expect(madeProgress).To(BeTrue())
		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].IsRead).To(BeFalse())
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(9))
	})

	It("should send read response after latency ticks", func() {
		topPort.Deliver(makeReadReq())

		// Tick 1: take request, CycleLeft: 10 → 9
		modelingtest.Tick(memController)

		// Ticks 2-9: count down (9 → 2)
		for i := 0; i < 8; i++ {
			modelingtest.Tick(memController)
		}

		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(1))

		// Tick 10: CycleLeft 1→0, then send response
		modelingtest.Tick(memController)

		state = memController.State
		Expect(state.InflightTransactions).To(HaveLen(0))

		rsp, _ := topPort.RetrieveOutgoing()
		Expect(rsp).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.DataReadyRsp{}}))
	})

	It("should send write response after latency ticks", func() {
		writeReq := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0,
			Data:    []byte{0, 1, 2, 3}},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote()}

		writeReq.TrafficBytes = len(writeReq.Payload.(memprotocol.WriteReq).Data) + 12
		writeReq.TrafficClass = "memprotocol.WriteReq"
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
		Expect(state.InflightTransactions).To(HaveLen(0))

		rsp, _ := topPort.RetrieveOutgoing()
		Expect(rsp).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.WriteDoneRsp{}}))

		// Verify data was written to storage
		data := storage.Read(0, 4)
		Expect(data).To(Equal([]byte{0, 1, 2, 3}))
	})

	It("should retry send when port is busy", func() {
		// Rebuild with a single-slot Top port so the outgoing buffer can be
		// forced full.
		build(1)

		// Pre-fill the outgoing buffer so the controller's response Send fails.
		dummy := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			Src:          topPort.AsRemote(),
			Dst:          messaging.RemotePort("Agent"),
			TrafficClass: "memprotocol.WriteDoneRsp"}

		topPort.Send(dummy)

		topPort.Deliver(makeReadReq())

		// Tick 1: take request, CycleLeft: 10 → 9
		modelingtest.Tick(memController)

		// Ticks 2-9: count down (8 ticks, 9→1)
		for i := 0; i < 8; i++ {
			modelingtest.Tick(memController)
		}

		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(1))

		// Tick 10: CycleLeft 1→0, send fails (outgoing buffer full)
		modelingtest.Tick(memController)

		state = memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(0))

		// Free the outgoing buffer, then retry succeeds.
		value0, present0 := topPort.RetrieveOutgoing()
		Expect(present0).To(BeTrue())
		Expect(value0).To(Equal(dummy))
		modelingtest.Tick(memController)

		state = memController.State
		Expect(state.InflightTransactions).To(HaveLen(0))
	})

	It("should write with dirty mask", func() {
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
		writeReq.TrafficClass = "memprotocol.WriteReq"
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
		Expect(data).To(Equal([]byte{10, 20, 2, 40}))
	})

	It("should use Spec for latency and width", func() {
		spec := memController.Spec
		Expect(spec.Latency).To(Equal(10))
		Expect(spec.Width).To(Equal(1))
	})

	It("should use State for current state", func() {
		state := memController.State
		Expect(state.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
	})
})
