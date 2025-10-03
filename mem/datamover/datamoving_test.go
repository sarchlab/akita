package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"go.uber.org/mock/gomock"
)

// type dataMoverLogger struct{}

// func (l *dataMoverLogger) StartTask(task tracing.Task) {
// 	fmt.Printf("Start task %+v\n", task)
// }

// func (l *dataMoverLogger) StepTask(task tracing.Task) {
// 	// Do nothing.
// }

// func (l *dataMoverLogger) EndTask(task tracing.Task) {
// 	fmt.Printf("End task %+v\n", task)
// }

var _ = Describe("DataMover", func() {
	var (
		mockCtrl  *gomock.Controller
		engine    sim.Engine
		dataMover *Comp
		// logger     *dataMoverLogger
		insideMem  *idealmemcontroller.Comp
		outsideMem *idealmemcontroller.Comp
		conn       *directconnection.Comp
		srcPort    *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = sim.NewSerialEngine()
		srcPort = NewMockPort(mockCtrl)
		srcPort.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		srcPort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		srcPort.EXPECT().AsRemote().Return(sim.RemotePort("SrcPort")).AnyTimes()
		insideMem = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithCacheLineSize(64).
			WithNewStorage(1 * mem.MB).
			Build("InsideMem")
		outsideMem = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithCacheLineSize(64).
			WithNewStorage(1 * mem.MB).
			Build("OutsideMem")
		dataMover = MakeBuilder().
			WithEngine(engine).
			WithBufferSize(2048).
			WithInsidePortMapper(&mem.SinglePortMapper{
				Port: insideMem.GetPortByName("Top").AsRemote(),
			}).
			WithOutsidePortMapper(&mem.SinglePortMapper{
				Port: outsideMem.GetPortByName("Top").AsRemote(),
			}).
			WithInsideByteGranularity(64).
			WithOutsideByteGranularity(256).
			Build("DataMover")

		// logger = new(dataMoverLogger)
		// tracing.CollectTrace(dataMover, logger)

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		conn.PlugIn(srcPort)
		conn.PlugIn(dataMover.ctrlPort)
		conn.PlugIn(dataMover.insidePort)
		conn.PlugIn(dataMover.outsidePort)
		conn.PlugIn(insideMem.GetPortByName("Top"))
		conn.PlugIn(outsideMem.GetPortByName("Top"))
	})

	It("should move data outside to inside", func() {
		data := make([]byte, 4096)
		for i := 0; i < 4096; i++ {
			data[i] = byte(i)
		}
		outsideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort.AsRemote()).
			WithDst(dataMover.ctrlPort.AsRemote()).
			WithSrcAddress(0).
			WithSrcSide("outside").
			WithDstAddress(0).
			WithDstSide("inside").
			WithByteSize(4096).
			Build()

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		Expect(insideMem.Storage.Read(0, 4096)).To(Equal(data))
	})

	It("should move data inside to outside", func() {
		data := make([]byte, 4096)
		for i := 0; i < 4096; i++ {
			data[i] = byte(i)
		}
		insideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort.AsRemote()).
			WithDst(dataMover.ctrlPort.AsRemote()).
			WithSrcAddress(0).
			WithSrcSide("inside").
			WithDstAddress(0).
			WithDstSide("outside").
			WithByteSize(4096).
			Build()

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		Expect(insideMem.Storage.Read(0, 4096)).To(Equal(data))
	})

	It("should move on difference addresses", func() {
		data := make([]byte, 4096)
		for i := 0; i < 4096; i++ {
			data[i] = byte(i)
		}
		insideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort.AsRemote()).
			WithDst(dataMover.ctrlPort.AsRemote()).
			WithSrcAddress(0).
			WithSrcSide("inside").
			WithDstAddress(4096).
			WithDstSide("outside").
			WithByteSize(4096).
			Build()

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		Expect(outsideMem.Storage.Read(4096, 4096)).To(Equal(data))
	})

	It("should move partial data", func() {
		data := make([]byte, 1024)
		for i := 0; i < 1024; i++ {
			data[i] = byte(i)
		}
		outsideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort.AsRemote()).
			WithDst(dataMover.ctrlPort.AsRemote()).
			WithSrcAddress(0).
			WithSrcSide("outside").
			WithDstAddress(512).
			WithDstSide("inside").
			WithByteSize(512).
			Build()

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		expected := data[:512]
		Expect(insideMem.Storage.Read(512, 512)).To(Equal(expected))
	})

	It("should handle zero-size transfers", func() {
		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort.AsRemote()).
			WithDst(dataMover.ctrlPort.AsRemote()).
			WithSrcAddress(0).
			WithSrcSide("inside").
			WithDstAddress(0).
			WithDstSide("outside").
			WithByteSize(0).
			Build()

		Expect(func() { dataMover.ctrlPort.Deliver(req) }).NotTo(Panic())
	})

	It("should handle overlapping ranges", func() {
		data := make([]byte, 1024)
		for i := 0; i < 1024; i++ {
			data[i] = byte(i)
		}
		insideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort.AsRemote()).
			WithDst(dataMover.ctrlPort.AsRemote()).
			WithSrcAddress(0).
			WithSrcSide("inside").
			WithDstAddress(512).
			WithDstSide("inside").
			WithByteSize(512).
			Build()

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		expected := append(data[:512], data[:512]...)
		Expect(insideMem.Storage.Read(0, 1024)).To(Equal(expected))
	})
})
