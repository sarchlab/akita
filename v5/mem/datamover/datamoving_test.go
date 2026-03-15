package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"go.uber.org/mock/gomock"
)

var _ = Describe("DataMover", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     sim.Engine
		dataMover  *modeling.Component[Spec, State]
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
			WithSpec(idealmemcontroller.Spec{
				Latency:       100,
				Width:         1,
				CacheLineSize: 64,
			}).
			WithNewStorage(1 * mem.MB).
			WithTopPort(sim.NewPort(nil, 16, 16, "InsideMem.TopPort")).
			WithCtrlPort(sim.NewPort(nil, 16, 16, "InsideMem.CtrlPort")).
			Build("InsideMem")
		outsideMem = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithSpec(idealmemcontroller.Spec{
				Latency:       100,
				Width:         1,
				CacheLineSize: 64,
			}).
			WithNewStorage(1 * mem.MB).
			WithTopPort(sim.NewPort(nil, 16, 16, "OutsideMem.TopPort")).
			WithCtrlPort(sim.NewPort(nil, 16, 16, "OutsideMem.CtrlPort")).
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
			WithCtrlPort(sim.NewPort(nil, 40960000, 40960000, "DataMover.CtrlPort")).
			WithInsidePort(sim.NewPort(nil, 64, 64, "DataMover.SrcPort")).
			WithOutsidePort(sim.NewPort(nil, 64, 64, "DataMover.DstPort")).
			Build("DataMover")

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		conn.PlugIn(srcPort)
		conn.PlugIn(dataMover.GetPortByName("Control"))
		conn.PlugIn(dataMover.GetPortByName("Inside"))
		conn.PlugIn(dataMover.GetPortByName("Outside"))
		conn.PlugIn(insideMem.GetPortByName("Top"))
		conn.PlugIn(outsideMem.GetPortByName("Top"))
	})

	It("should move data outside to inside", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideMem.GetStorage().Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.Any())

		req := &DataMoveRequest{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Control").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 4096
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Control").Deliver(req)

		engine.Run()

		Expect(insideMem.GetStorage().Read(0, 4096)).To(Equal(data))
	})

	It("should move data inside to outside", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideMem.GetStorage().Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.Any())

		req := &DataMoveRequest{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Control").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 0
		req.DstSide = "outside"
		req.ByteSize = 4096
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Control").Deliver(req)

		engine.Run()

		Expect(insideMem.GetStorage().Read(0, 4096)).To(Equal(data))
	})

	It("should move on difference addresses", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideMem.GetStorage().Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.Any())

		req := &DataMoveRequest{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Control").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 4096
		req.DstSide = "outside"
		req.ByteSize = 4096
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Control").Deliver(req)

		engine.Run()

		Expect(outsideMem.GetStorage().Read(4096, 4096)).To(Equal(data))
	})

	It("should move partial data", func() {
		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		outsideMem.GetStorage().Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.Any())

		req := &DataMoveRequest{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Control").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 512
		req.DstSide = "inside"
		req.ByteSize = 512
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Control").Deliver(req)

		engine.Run()

		expected := data[:512]
		Expect(insideMem.GetStorage().Read(512, 512)).To(Equal(expected))
	})

	It("should handle zero-size transfers", func() {
		req := &DataMoveRequest{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Control").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 0
		req.DstSide = "outside"
		req.ByteSize = 0
		req.TrafficClass = "datamover.DataMoveRequest"

		Expect(func() {
			dataMover.GetPortByName("Control").Deliver(req)
		}).NotTo(Panic())
	})

	It("should handle overlapping ranges", func() {
		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		insideMem.GetStorage().Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.Any())

		req := &DataMoveRequest{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Control").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 512
		req.DstSide = "inside"
		req.ByteSize = 512
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Control").Deliver(req)

		engine.Run()

		expected := append(data[:512], data[:512]...)
		Expect(insideMem.GetStorage().Read(0, 1024)).To(Equal(expected))
	})
})
