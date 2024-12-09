package datamover

import (
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/tracing"
)

type dataMoverLogger struct{}

func (l *dataMoverLogger) StartTask(task tracing.Task) {
	fmt.Printf("Start task %+v\n", task)
}

func (l *dataMoverLogger) StepTask(task tracing.Task) {
	// Do nothing.
}

func (l *dataMoverLogger) EndTask(task tracing.Task) {
	fmt.Printf("End task %+v\n", task)
}

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
				Port: insideMem.GetPortByName("Top"),
			}).
			WithOutsidePortMapper(&mem.SinglePortMapper{
				Port: outsideMem.GetPortByName("Top"),
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
		conn.PlugIn(srcPort, 64)
		conn.PlugIn(dataMover.ctrlPort, 64)
		conn.PlugIn(dataMover.insidePort, 64)
		conn.PlugIn(dataMover.outsidePort, 64)
		conn.PlugIn(insideMem.GetPortByName("Top"), 64)
		conn.PlugIn(outsideMem.GetPortByName("Top"), 64)
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
			WithSrc(srcPort).
			WithDst(dataMover.ctrlPort).
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
			WithSrc(srcPort).
			WithDst(dataMover.ctrlPort).
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

	FIt("should move on difference addresses", func() {
		data := make([]byte, 4096)
		for i := 0; i < 4096; i++ {
			data[i] = byte(i)
		}
		insideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort).
			WithDst(dataMover.ctrlPort).
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
})
