package idealmemcontroller

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"

	. "github.com/onsi/gomega"
)

var _ = Describe("Ideal Memory Controller", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		simulation    *MockSimulation
		memController *Comp
		port          *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		simulation = NewMockSimulation(mockCtrl)
		simulation.EXPECT().GetEngine().Return(engine).AnyTimes()
		simulation.EXPECT().
			RegisterStateHolder(gomock.Any()).
			Return().
			AnyTimes()

		port = NewMockPort(mockCtrl)
		port.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("Port")).
			AnyTimes()

		memController = MakeBuilder().
			WithSimulation(simulation).
			WithNewStorage(1 * mem.MB).
			Build("MemCtrl")
		memController.Freq = 1000 * timing.MHz
		memController.Latency = 10
		memController.topPort = port
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should process read request", func() {
		readReq := mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:            0,
			AccessByteSize:     4,
			CanWaitForCoalesce: false,
		}
		port.EXPECT().RetrieveIncoming().Return(readReq)
		engine.EXPECT().Now().Return(timing.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
	})

	It("should process write request", func() {
		writeReq := mem.WriteReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:   0,
			Data:      []byte{0, 1, 2, 3},
			DirtyMask: []bool{false, false, true, false},
		}
		port.EXPECT().RetrieveIncoming().Return(writeReq)
		engine.EXPECT().Now().Return(timing.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle read respond event", func() {
		data := []byte{1, 2, 3, 4}
		memController.Storage.Write(0, data)

		readReq := mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:            0,
			AccessByteSize:     4,
			CanWaitForCoalesce: false,
		}

		event := newReadRespondEvent(11, memController, readReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(mem.DataReadyRsp{}))
		engine.EXPECT().Now().Return(timing.VTimeInSec(10))

		memController.Handle(event)
	})

	It("should retry read if send DataReady failed", func() {
		data := []byte{1, 2, 3, 4}
		memController.Storage.Write(0, data)

		readReq := mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:            0,
			AccessByteSize:     4,
			CanWaitForCoalesce: false,
		}
		event := newReadRespondEvent(11, memController, readReq)

		port.EXPECT().
			Send(gomock.AssignableToTypeOf(mem.DataReadyRsp{})).
			Return(&modeling.SendError{})

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		memController.Handle(event)
	})

	It("should handle write respond event without write mask", func() {
		data := []byte{1, 2, 3, 4}
		writeReq := mem.WriteReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:   0,
			Data:      data,
			DirtyMask: nil,
		}
		event := newWriteRespondEvent(11, memController, writeReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(mem.WriteDoneRsp{}))
		engine.EXPECT().Now().Return(timing.VTimeInSec(10))

		memController.Handle(event)

		retData, _ := memController.Storage.Read(0, 4)
		Expect(retData).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should handle write respond event", func() {
		memController.Storage.Write(0, []byte{9, 9, 9, 9})
		data := []byte{1, 2, 3, 4}
		dirtyMask := []bool{false, true, false, false}

		writeReq := mem.WriteReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:   0,
			Data:      data,
			DirtyMask: dirtyMask,
		}
		event := newWriteRespondEvent(11, memController, writeReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(mem.WriteDoneRsp{}))
		engine.EXPECT().Now().Return(timing.VTimeInSec(10))

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
		writeReq := mem.WriteReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:   0,
			Data:      data,
			DirtyMask: dirtyMask,
		}

		event := newWriteRespondEvent(11, memController, writeReq)
		engine.EXPECT().Schedule(gomock.Any()).AnyTimes()
		port.EXPECT().
			Send(gomock.AssignableToTypeOf(mem.WriteDoneRsp{})).
			AnyTimes()

		b.Time("write time", func() {
			for i := 0; i < 100000; i++ {
				memController.Handle(event)
			}
		})
	}, 100)

	It("should retry write respond event, if network busy", func() {
		data := []byte{1, 2, 3, 4}

		writeReq := mem.WriteReq{
			MsgMeta: modeling.MsgMeta{
				Src: port.AsRemote(),
				Dst: memController.topPort.AsRemote(),
				ID:  id.Generate(),
			},
			Address:   0,
			Data:      data,
			DirtyMask: nil,
		}
		event := newWriteRespondEvent(11, memController, writeReq)

		port.EXPECT().
			Send(gomock.AssignableToTypeOf(mem.WriteDoneRsp{})).
			Return(&modeling.SendError{})
		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		memController.Handle(event)
	})
})
