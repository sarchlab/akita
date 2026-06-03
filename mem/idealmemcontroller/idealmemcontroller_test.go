package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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

var _ = Describe("Ideal Memory Controller", func() {
	var (
		engine        timing.Engine
		storage       *mem.Storage
		memController *Comp
		topPort       messaging.Port
	)

	// build constructs a controller with the given Top-port buffer size, injects
	// the shared storage, and plugs a noopConn so its ports can be driven.
	build := func(topBufSize int) {
		spec := DefaultSpec()
		spec.Width = 1
		spec.Latency = 10
		spec.CacheLineSize = 64
		spec.TopPortBufferSize = topBufSize

		memController = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{Storage: storage}).
			WithSpec(spec).
			Build("MemCtrl")

		topPort = memController.GetPortByName("Top")
		conn := &noopConn{}
		conn.PlugIn(topPort)
	}

	makeReadReq := func() *mem.ReadReq {
		req := &mem.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = 0
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(1 * mem.MB)
		build(16)
	})

	It("should accept read request and add to inflight transactions", func() {
		topPort.Deliver(makeReadReq())

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].IsRead).To(BeTrue())
		// After first tick: latency=10, decrement once → 9
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(9))
	})

	It("should accept write request and add to inflight transactions", func() {
		writeReq := &mem.WriteReq{}
		writeReq.ID = timing.GetIDGenerator().Generate()
		writeReq.Src = messaging.RemotePort("Agent")
		writeReq.Dst = topPort.AsRemote()
		writeReq.Address = 0
		writeReq.Data = []byte{0, 1, 2, 3}
		writeReq.DirtyMask = []bool{false, false, true, false}
		writeReq.TrafficBytes = len(writeReq.Data) + 12
		writeReq.TrafficClass = "mem.WriteReq"
		topPort.Deliver(writeReq)

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].IsRead).To(BeFalse())
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(9))
	})

	It("should send read response after latency ticks", func() {
		topPort.Deliver(makeReadReq())

		// Tick 1: take request, CycleLeft: 10 → 9
		memController.Tick()

		// Ticks 2-9: count down (9 → 2)
		for i := 0; i < 8; i++ {
			memController.Tick()
		}

		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(1))

		// Tick 10: CycleLeft 1→0, then send response
		memController.Tick()

		state = memController.State
		Expect(state.InflightTransactions).To(HaveLen(0))

		rsp := topPort.RetrieveOutgoing()
		Expect(rsp).To(BeAssignableToTypeOf(&mem.DataReadyRsp{}))
	})

	It("should send write response after latency ticks", func() {
		writeReq := &mem.WriteReq{}
		writeReq.ID = timing.GetIDGenerator().Generate()
		writeReq.Src = messaging.RemotePort("Agent")
		writeReq.Dst = topPort.AsRemote()
		writeReq.Address = 0
		writeReq.Data = []byte{0, 1, 2, 3}
		writeReq.TrafficBytes = len(writeReq.Data) + 12
		writeReq.TrafficClass = "mem.WriteReq"
		topPort.Deliver(writeReq)

		// Tick 1: take request, CycleLeft: 10 → 9
		memController.Tick()

		// Ticks 2-9: count down
		for i := 0; i < 8; i++ {
			memController.Tick()
		}

		// Tick 10: CycleLeft 1→0, send response
		memController.Tick()

		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(0))

		rsp := topPort.RetrieveOutgoing()
		Expect(rsp).To(BeAssignableToTypeOf(&mem.WriteDoneRsp{}))

		// Verify data was written to storage
		data, err := storage.Read(0, 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal([]byte{0, 1, 2, 3}))
	})

	It("should retry send when port is busy", func() {
		// Rebuild with a single-slot Top port so the outgoing buffer can be
		// forced full.
		build(1)

		// Pre-fill the outgoing buffer so the controller's response Send fails.
		dummy := &mem.WriteDoneRsp{}
		dummy.Src = topPort.AsRemote()
		dummy.Dst = messaging.RemotePort("Agent")
		dummy.TrafficClass = "mem.WriteDoneRsp"
		topPort.Send(dummy)

		topPort.Deliver(makeReadReq())

		// Tick 1: take request, CycleLeft: 10 → 9
		memController.Tick()

		// Ticks 2-9: count down (8 ticks, 9→1)
		for i := 0; i < 8; i++ {
			memController.Tick()
		}

		state := memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(1))

		// Tick 10: CycleLeft 1→0, send fails (outgoing buffer full)
		memController.Tick()

		state = memController.State
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(0))

		// Free the outgoing buffer, then retry succeeds.
		Expect(topPort.RetrieveOutgoing()).To(Equal(dummy))
		memController.Tick()

		state = memController.State
		Expect(state.InflightTransactions).To(HaveLen(0))
	})

	It("should write with dirty mask", func() {
		// Pre-write data
		err := storage.Write(0, []byte{10, 20, 30, 40})
		Expect(err).ToNot(HaveOccurred())

		writeReq := &mem.WriteReq{}
		writeReq.ID = timing.GetIDGenerator().Generate()
		writeReq.Src = messaging.RemotePort("Agent")
		writeReq.Dst = topPort.AsRemote()
		writeReq.Address = 0
		writeReq.Data = []byte{0, 1, 2, 3}
		writeReq.DirtyMask = []bool{false, false, true, false}
		writeReq.TrafficBytes = len(writeReq.Data) + 12
		writeReq.TrafficClass = "mem.WriteReq"
		topPort.Deliver(writeReq)

		// Tick 1: take request
		memController.Tick()

		// Ticks 2-9: count down
		for i := 0; i < 8; i++ {
			memController.Tick()
		}

		// Tick 10: send response
		memController.Tick()

		// Check that only dirty bytes were written
		data, err := storage.Read(0, 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal([]byte{10, 20, 2, 40}))
	})

	It("should use Spec for latency and width", func() {
		spec := memController.Spec()
		Expect(spec.Latency).To(Equal(10))
		Expect(spec.Width).To(Equal(1))
	})

	It("should use State for current state", func() {
		state := memController.State
		Expect(state.CurrentState).To(Equal("enable"))
	})
})
