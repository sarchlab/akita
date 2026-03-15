package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v5/mem"
	"go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/gomega"
)

var _ = Describe("Ideal Memory Controller", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		memController *Comp
		port          *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		port = NewMockPort(mockCtrl)
		port.EXPECT().
			AsRemote().
			Return(sim.RemotePort("Port")).
			AnyTimes()
		port.EXPECT().
			SetComponent(gomock.Any()).
			AnyTimes()

		memController = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			WithSpec(Spec{Width: 1, Latency: 10, CacheLineSize: 64}).
			WithTopPort(port).
			WithCtrlPort(sim.NewPort(nil, 16, 16, "MemCtrl.CtrlPort")).
			Build("MemCtrl")
		memController.Freq = 1000 * sim.MHz
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should accept read request and add to inflight transactions", func() {
		readReq := &mem.ReadReq{}
		readReq.ID = sim.GetIDGenerator().Generate()
		readReq.Dst = port.AsRemote()
		readReq.Address = 0
		readReq.AccessByteSize = 4
		readReq.TrafficBytes = 12
		readReq.TrafficClass = "mem.ReadReq"
		port.EXPECT().RetrieveIncoming().Return(readReq)

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
		state := memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].IsRead).To(BeTrue())
		// After first tick: latency=10, decrement once → 9
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(9))
	})

	It("should accept write request and add to inflight transactions", func() {
		writeData1 := []byte{0, 1, 2, 3}
		writeReq := &mem.WriteReq{}
		writeReq.ID = sim.GetIDGenerator().Generate()
		writeReq.Dst = port.AsRemote()
		writeReq.Address = 0
		writeReq.Data = writeData1
		writeReq.DirtyMask = []bool{false, false, true, false}
		writeReq.TrafficBytes = len(writeData1) + 12
		writeReq.TrafficClass = "mem.WriteReq"
		port.EXPECT().RetrieveIncoming().Return(writeReq)

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
		state := memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].IsRead).To(BeFalse())
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(9))
	})

	It("should send read response after latency ticks", func() {
		readReq := &mem.ReadReq{}
		readReq.ID = sim.GetIDGenerator().Generate()
		readReq.Dst = port.AsRemote()
		readReq.Address = 0
		readReq.AccessByteSize = 4
		readReq.TrafficBytes = 12
		readReq.TrafficClass = "mem.ReadReq"

		// Tick 1: take request, CycleLeft: 10 → 9
		port.EXPECT().RetrieveIncoming().Return(readReq)
		memController.Tick()

		// Ticks 2-9: count down (9 → 2)
		for i := 0; i < 8; i++ {
			port.EXPECT().RetrieveIncoming().Return(nil)
			memController.Tick()
		}

		state := memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(1))

		// Tick 10: CycleLeft 1→0, then send response
		port.EXPECT().RetrieveIncoming().Return(nil)
		port.EXPECT().Send(gomock.Any()).Return(nil)
		memController.Tick()

		state = memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(0))
	})

	It("should send write response after latency ticks", func() {
		writeData2 := []byte{0, 1, 2, 3}
		writeReq := &mem.WriteReq{}
		writeReq.ID = sim.GetIDGenerator().Generate()
		writeReq.Dst = port.AsRemote()
		writeReq.Address = 0
		writeReq.Data = writeData2
		writeReq.TrafficBytes = len(writeData2) + 12
		writeReq.TrafficClass = "mem.WriteReq"

		// Tick 1: take request, CycleLeft: 10 → 9
		port.EXPECT().RetrieveIncoming().Return(writeReq)
		memController.Tick()

		// Ticks 2-9: count down
		for i := 0; i < 8; i++ {
			port.EXPECT().RetrieveIncoming().Return(nil)
			memController.Tick()
		}

		// Tick 10: CycleLeft 1→0, send response
		port.EXPECT().RetrieveIncoming().Return(nil)
		port.EXPECT().Send(gomock.Any()).Return(nil)
		memController.Tick()

		state := memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(0))

		// Verify data was written to storage
		data, err := memController.GetStorage().Read(0, 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal([]byte{0, 1, 2, 3}))
	})

	It("should retry send when port is busy", func() {
		readReq := &mem.ReadReq{}
		readReq.ID = sim.GetIDGenerator().Generate()
		readReq.Dst = port.AsRemote()
		readReq.Address = 0
		readReq.AccessByteSize = 4
		readReq.TrafficBytes = 12
		readReq.TrafficClass = "mem.ReadReq"

		// Tick 1: take request, CycleLeft: 10 → 9
		port.EXPECT().RetrieveIncoming().Return(readReq)
		memController.Tick()

		// Ticks 2-9: count down (8 ticks, 9→2)
		for i := 0; i < 8; i++ {
			port.EXPECT().RetrieveIncoming().Return(nil)
			memController.Tick()
		}

		// Tick 10: CycleLeft 1→0, send fails
		port.EXPECT().RetrieveIncoming().Return(nil)
		port.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})
		memController.Tick()

		state := memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(1))
		Expect(state.InflightTransactions[0].CycleLeft).To(Equal(0))

		// Tick 11: Retry succeeds (CycleLeft stays 0, attempts send again)
		port.EXPECT().RetrieveIncoming().Return(nil)
		port.EXPECT().Send(gomock.Any()).Return(nil)
		memController.Tick()

		state = memController.Component.GetState()
		Expect(state.InflightTransactions).To(HaveLen(0))
	})

	It("should write with dirty mask", func() {
		// Pre-write data
		err := memController.GetStorage().Write(0, []byte{10, 20, 30, 40})
		Expect(err).ToNot(HaveOccurred())

		writeData3 := []byte{0, 1, 2, 3}
		writeReq := &mem.WriteReq{}
		writeReq.ID = sim.GetIDGenerator().Generate()
		writeReq.Dst = port.AsRemote()
		writeReq.Address = 0
		writeReq.Data = writeData3
		writeReq.DirtyMask = []bool{false, false, true, false}
		writeReq.TrafficBytes = len(writeData3) + 12
		writeReq.TrafficClass = "mem.WriteReq"

		// Tick 1: take request
		port.EXPECT().RetrieveIncoming().Return(writeReq)
		memController.Tick()

		// Ticks 2-9: count down
		for i := 0; i < 8; i++ {
			port.EXPECT().RetrieveIncoming().Return(nil)
			memController.Tick()
		}

		// Tick 10: send response
		port.EXPECT().RetrieveIncoming().Return(nil)
		port.EXPECT().Send(gomock.Any()).Return(nil)
		memController.Tick()

		// Check that only dirty bytes were written
		data, err := memController.GetStorage().Read(0, 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal([]byte{10, 20, 2, 40}))
	})

	It("should use Spec for latency and width", func() {
		spec := memController.Component.GetSpec()
		Expect(spec.Latency).To(Equal(10))
		Expect(spec.Width).To(Equal(1))
	})

	It("should use State for current state", func() {
		state := memController.Component.GetState()
		Expect(state.CurrentState).To(Equal("enable"))
	})
})
