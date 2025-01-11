package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
	"go.uber.org/mock/gomock"
)

var _ = Describe("DataMover", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     timing.Engine
		sim        simulation.Simulation
		dataMover  *Comp
		insideMem  *idealmemcontroller.Comp
		outsideMem *idealmemcontroller.Comp
		conn       *directconnection.Comp
		srcPort    *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = timing.NewSerialEngine()
		sim = simulation.NewSimulation()
		sim.RegisterEngine(engine)

		srcPort = NewMockPort(mockCtrl)
		srcPort.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		srcPort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		srcPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("SrcPort")).
			AnyTimes()
		insideMem = idealmemcontroller.MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).
			WithCacheLineSize(64).
			WithNewStorage(1 * mem.MB).
			Build("InsideMem")
		outsideMem = idealmemcontroller.MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).
			WithCacheLineSize(64).
			WithNewStorage(1 * mem.MB).
			Build("OutsideMem")
		dataMover = MakeBuilder().
			WithSimulation(sim).
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

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * timing.GHz).
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
			Deliver(gomock.AssignableToTypeOf(modeling.GeneralRsp{}))

		req := DataMoveRequest{
			MsgMeta: modeling.MsgMeta{
				Src: srcPort.AsRemote(),
				Dst: dataMover.ctrlPort.AsRemote(),
			},
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   4096,
		}
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
			Deliver(gomock.AssignableToTypeOf(modeling.GeneralRsp{}))

		req := DataMoveRequest{
			MsgMeta: modeling.MsgMeta{
				Src: srcPort.AsRemote(),
				Dst: dataMover.ctrlPort.AsRemote(),
			},
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 0,
			DstSide:    "outside",
			ByteSize:   4096,
		}

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
			Deliver(gomock.AssignableToTypeOf(modeling.GeneralRsp{}))

		req := DataMoveRequest{
			MsgMeta: modeling.MsgMeta{
				Src: srcPort.AsRemote(),
				Dst: dataMover.ctrlPort.AsRemote(),
			},
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 4096,
			DstSide:    "outside",
			ByteSize:   4096,
		}

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		Expect(outsideMem.Storage.Read(4096, 4096)).To(Equal(data))
	})
})
