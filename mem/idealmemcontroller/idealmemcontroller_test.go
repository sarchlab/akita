package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v4/mem/mem"
	"go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v4/sim"

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

		memController = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			Build("MemCtrl")
		memController.Freq = 1000 * sim.MHz
		memController.Latency = 10
		memController.topPort = port
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should process read request", func() {
		readReq := mem.ReadReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithByteSize(4).
			Build()
		port.EXPECT().RetrieveIncoming().Return(readReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
	})

	It("should process write request", func() {
		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithData([]byte{0, 1, 2, 3}).
			WithDirtyMask([]bool{false, false, true, false}).
			Build()
		port.EXPECT().RetrieveIncoming().Return(writeReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle read respond event", func() {
		data := []byte{1, 2, 3, 4}
		memController.Storage.Write(0, data)

		readReq := mem.ReadReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithByteSize(4).
			Build()

		event := newReadRespondEvent(11, memController, readReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{}))
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		memController.Handle(event)
	})

	It("should retry read if send DataReady failed", func() {
		data := []byte{1, 2, 3, 4}
		memController.Storage.Write(0, data)

		readReq := mem.ReadReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithByteSize(4).
			Build()
		event := newReadRespondEvent(11, memController, readReq)

		port.EXPECT().
			Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
			Return(&sim.SendError{})

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		memController.Handle(event)
	})

	It("should handle write respond event without write mask", func() {
		data := []byte{1, 2, 3, 4}
		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithData(data).
			Build()
		event := newWriteRespondEvent(11, memController, writeReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{}))
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		memController.Handle(event)

		retData, _ := memController.Storage.Read(0, 4)
		Expect(retData).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should handle write respond event", func() {
		memController.Storage.Write(0, []byte{9, 9, 9, 9})
		data := []byte{1, 2, 3, 4}
		dirtyMask := []bool{false, true, false, false}

		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithData(data).
			WithDirtyMask(dirtyMask).
			Build()
		event := newWriteRespondEvent(11, memController, writeReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{}))
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		memController.Handle(event)
		retData, _ := memController.Storage.Read(0, 4)
		Expect(retData).To(Equal([]byte{9, 2, 9, 9}))
	})

	Measure("write with write mask", func(b Benchmarker) {
		data := make([]byte, 64)
		dirtyMask := []bool{
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
		}
		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithData(data).
			WithDirtyMask(dirtyMask).
			Build()

		event := newWriteRespondEvent(11, memController, writeReq)
		engine.EXPECT().Schedule(gomock.Any()).AnyTimes()
		port.EXPECT().
			Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{})).
			AnyTimes()

		b.Time("write time", func() {
			for i := 0; i < 100000; i++ {
				memController.Handle(event)
			}
		})
	}, 100)

	It("should retry write respond event, if network busy", func() {
		data := []byte{1, 2, 3, 4}

		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithData(data).
			Build()
		event := newWriteRespondEvent(11, memController, writeReq)

		port.EXPECT().
			Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{})).
			Return(&sim.SendError{})
		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		memController.Handle(event)
	})
})
