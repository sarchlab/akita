package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("DataMover", func() {
	var (
		engine         timing.Engine
		sim            timing.Simulation
		dataMover      *Comp
		insideMem      *idealmemcontroller.Comp
		insideStorage  *mem.Storage
		outsideMem     *idealmemcontroller.Comp
		outsideStorage *mem.Storage
		conn           *directconnection.Comp
		srcPort        messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		srcPort = messaging.NewPort("Src.Top", 4, 4)

		memSpec := idealmemcontroller.Definition.DefaultSpec
		memSpec.Latency = 100
		memSpec.Width = 1
		memSpec.CacheLineSize = 64

		insideStorage = mem.NewStorage(1 * mem.MB)
		insideMem = buildIdealMem(sim, memSpec, insideStorage, "InsideMem")
		outsideStorage = mem.NewStorage(1 * mem.MB)
		outsideMem = buildIdealMem(sim, memSpec, outsideStorage, "OutsideMem")

		dmSpec := Definition.DefaultSpec
		dmSpec.BufferSize = 2048
		dmSpec.InsideByteGranularity = 64
		dmSpec.OutsideByteGranularity = 256

		dataMover = Definition.Builder().
			WithSimulation(sim).
			WithSpec(dmSpec).
			WithResources(Resources{
				InsideMapper: &mem.SinglePortMapper{
					Port: insideMem.Ports.Top.AsRemote(),
				},
				OutsideMapper: &mem.SinglePortMapper{
					Port: outsideMem.Ports.Top.AsRemote(),
				},
			}).
			WithPorts(makePorts("DataMover", 16, 64, 64, 40960000)).
			Build("DataMover")

		conn = directconnection.MakeBuilder().
			WithSimulation(sim).
			Build("Conn")
		conn.PlugIn(srcPort)
		conn.PlugIn(dataMover.Ports.Top)
		conn.PlugIn(dataMover.Ports.Inside)
		conn.PlugIn(dataMover.Ports.Outside)
		conn.PlugIn(insideMem.Ports.Top)
		conn.PlugIn(outsideMem.Ports.Top)
	})

	It("should move data outside to inside", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 4096
		req.TrafficClass = "datamoverprotocol.datamoverprotocol.DataMoveRequest"

		dataMover.Ports.Top.Deliver(req)

		Expect(engine.Run()).To(Succeed())

		Expect(insideStorage.Read(0, 4096)).To(Equal(data))
		value0, present0 := srcPort.RetrieveIncoming()
		Expect(present0).To(BeTrue())
		Expect(value0).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
	})

	It("should move data when SrcAddress is beyond BufferSize", func() {
		// Regression: the read-window check must use the transaction-relative
		// address. With a source address at or beyond BufferSize (2048 here),
		// an absolute-address check would reject every read and the move would
		// hang. Source and destination addresses also differ.
		data := make([]byte, 2048)
		for i := range 2048 {
			data[i] = byte(i)
		}
		outsideStorage.Write(4096, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 4096
		req.SrcSide = "outside"
		req.DstAddress = 8192
		req.DstSide = "inside"
		req.ByteSize = 2048
		req.TrafficClass = "datamoverprotocol.DataMoveRequest"

		dataMover.Ports.Top.Deliver(req)

		Expect(engine.Run()).To(Succeed())

		Expect(insideStorage.Read(8192, 2048)).To(Equal(data))
		value1, present1 := srcPort.RetrieveIncoming()
		Expect(present1).To(BeTrue())
		Expect(value1).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
	})

	It("should move data inside to outside", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 0
		req.DstSide = "outside"
		req.ByteSize = 4096
		req.TrafficClass = "datamoverprotocol.datamoverprotocol.DataMoveRequest"

		dataMover.Ports.Top.Deliver(req)

		Expect(engine.Run()).To(Succeed())

		Expect(insideStorage.Read(0, 4096)).To(Equal(data))
		value2, present2 := srcPort.RetrieveIncoming()
		Expect(present2).To(BeTrue())
		Expect(value2).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
	})

	It("should move on difference addresses", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 4096
		req.DstSide = "outside"
		req.ByteSize = 4096
		req.TrafficClass = "datamoverprotocol.datamoverprotocol.DataMoveRequest"

		dataMover.Ports.Top.Deliver(req)

		Expect(engine.Run()).To(Succeed())

		Expect(outsideStorage.Read(4096, 4096)).To(Equal(data))
		value3, present3 := srcPort.RetrieveIncoming()
		Expect(present3).To(BeTrue())
		Expect(value3).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
	})

	It("should move partial data", func() {
		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 512
		req.DstSide = "inside"
		req.ByteSize = 512
		req.TrafficClass = "datamoverprotocol.datamoverprotocol.DataMoveRequest"

		dataMover.Ports.Top.Deliver(req)

		Expect(engine.Run()).To(Succeed())

		expected := data[:512]
		Expect(insideStorage.Read(512, 512)).To(Equal(expected))
		value4, present4 := srcPort.RetrieveIncoming()
		Expect(present4).To(BeTrue())
		Expect(value4).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
	})

	It("should handle zero-size transfers", func() {
		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 0
		req.DstSide = "outside"
		req.ByteSize = 0
		req.TrafficClass = "datamoverprotocol.datamoverprotocol.DataMoveRequest"

		Expect(func() {
			dataMover.Ports.Top.Deliver(req)
		}).NotTo(Panic())
	})

	It("should handle overlapping ranges", func() {
		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = sim.NewID()
		req.Src = srcPort.AsRemote()
		req.Dst = dataMover.Ports.Top.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "inside"
		req.DstAddress = 512
		req.DstSide = "inside"
		req.ByteSize = 512
		req.TrafficClass = "datamoverprotocol.datamoverprotocol.DataMoveRequest"

		dataMover.Ports.Top.Deliver(req)

		Expect(engine.Run()).To(Succeed())

		expected := append(data[:512], data[:512]...)
		Expect(insideStorage.Read(0, 1024)).To(Equal(expected))
		value5, present5 := srcPort.RetrieveIncoming()
		Expect(present5).To(BeTrue())
		Expect(value5).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
	})
})
