package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("DataMover", func() {
	var (
		engine         timing.Engine
		dataMover      *modeling.Component[Spec, State, modeling.None]
		insideMem      *idealmemcontroller.Comp
		insideStorage  *mem.Storage
		outsideMem     *idealmemcontroller.Comp
		outsideStorage *mem.Storage
		conn           *directconnection.Comp
		srcPort        messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		srcPort = messaging.NewPort(nil, 4, 4, "SrcPort")

		memSpec := idealmemcontroller.DefaultSpec()
		memSpec.Latency = 100
		memSpec.Width = 1
		memSpec.CacheLineSize = 64

		insideStorage = mem.NewStorage(1 * mem.MB)
		insideMem = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(memSpec).
			WithResources(idealmemcontroller.Resources{Storage: insideStorage}).
			Build("InsideMem")
		insideMem.AssignPort("Top",
			messaging.NewPort(insideMem, 16, 16, insideMem.Name()+".Top"))
		insideMem.AssignPort("Control",
			messaging.NewPort(insideMem, 16, 16, insideMem.Name()+".Control"))
		outsideStorage = mem.NewStorage(1 * mem.MB)
		outsideMem = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(memSpec).
			WithResources(idealmemcontroller.Resources{Storage: outsideStorage}).
			Build("OutsideMem")
		outsideMem.AssignPort("Top",
			messaging.NewPort(outsideMem, 16, 16, outsideMem.Name()+".Top"))
		outsideMem.AssignPort("Control",
			messaging.NewPort(outsideMem, 16, 16, outsideMem.Name()+".Control"))

		dmSpec := DefaultSpec()
		dmSpec.BufferSize = 2048
		dmSpec.InsideByteGranularity = 64
		dmSpec.OutsideByteGranularity = 256
		dmSpec.CtrlPortBufferSize = 40960000
		dmSpec.InsidePortBufferSize = 64
		dmSpec.OutsidePortBufferSize = 64

		dataMover = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(dmSpec).
			WithResources(Resources{
				InsideMapper: &mem.SinglePortMapper{
					Port: insideMem.GetPortByName("Top").AsRemote(),
				},
				OutsideMapper: &mem.SinglePortMapper{
					Port: outsideMem.GetPortByName("Top").AsRemote(),
				},
			}).
			Build("DataMover")

		conn = directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Conn")
		conn.PlugIn(srcPort)
		conn.PlugIn(dataMover.GetPortByName("Top"))
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
		outsideStorage.Write(0, data)

		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Top").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 4096
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Top").Deliver(req)

		engine.Run()

		Expect(insideStorage.Read(0, 4096)).To(Equal(data))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(DataMoveResponse{}))
	})

	It("should move data inside to outside", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Top").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 0
		req.DstSide = "outside"
		req.ByteSize = 4096
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Top").Deliver(req)

		engine.Run()

		Expect(insideStorage.Read(0, 4096)).To(Equal(data))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(DataMoveResponse{}))
	})

	It("should move on difference addresses", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Top").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 4096
		req.DstSide = "outside"
		req.ByteSize = 4096
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Top").Deliver(req)

		engine.Run()

		Expect(outsideStorage.Read(4096, 4096)).To(Equal(data))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(DataMoveResponse{}))
	})

	It("should move partial data", func() {
		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Top").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 512
		req.DstSide = "inside"
		req.ByteSize = 512
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Top").Deliver(req)

		engine.Run()

		expected := data[:512]
		Expect(insideStorage.Read(512, 512)).To(Equal(expected))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(DataMoveResponse{}))
	})

	It("should handle zero-size transfers", func() {
		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Top").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 0
		req.DstSide = "outside"
		req.ByteSize = 0
		req.TrafficClass = "datamover.DataMoveRequest"

		Expect(func() {
			dataMover.GetPortByName("Top").Deliver(req)
		}).NotTo(Panic())
	})

	It("should handle overlapping ranges", func() {
		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.GetPortByName("Top").AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 512
		req.DstSide = "inside"
		req.ByteSize = 512
		req.TrafficClass = "datamover.DataMoveRequest"

		dataMover.GetPortByName("Top").Deliver(req)

		engine.Run()

		expected := append(data[:512], data[:512]...)
		Expect(insideStorage.Read(0, 1024)).To(Equal(expected))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(DataMoveResponse{}))
	})
})
