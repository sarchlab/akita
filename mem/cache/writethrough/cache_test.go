package writethrough_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"

	. "github.com/sarchlab/akita/v4/mem/cache/writethrough"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"

	"github.com/sarchlab/akita/v4/mem/mem"
)

var _ = Describe("Cache", func() {
	var (
		mockCtrl            *gomock.Controller
		engine              sim.Engine
		connection          sim.Connection
		addressToPortMapper mem.AddressToPortMapper
		dram                *idealmemcontroller.Comp
		cuPort              *MockPort
		c                   *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cuPort = NewMockPort(mockCtrl)
		cuPort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		cuPort.EXPECT().AsRemote().Return(sim.RemotePort("CuPort")).AnyTimes()

		engine = sim.NewSerialEngine()
		connection = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		dram = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithNewStorage(4 * mem.GB).
			Build("DRAM")
		addressToPortMapper = &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}
		c = MakeBuilder().
			WithEngine(engine).
			WithAddressToPortMapper(addressToPortMapper).
			Build("Cache")

		connection.PlugIn(dram.GetPortByName("Top"))
		connection.PlugIn(c.GetPortByName("Top"))
		connection.PlugIn(c.GetPortByName("Bottom"))
		cuPort.EXPECT().SetConnection(connection)
		connection.PlugIn(cuPort)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do read miss", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4})
		read := mem.ReadReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x100).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Deliver(read)

		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})

		engine.Run()
	})

	It("should do read miss coalesce", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.ReadReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x100).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Deliver(read1)

		read2 := mem.ReadReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Deliver(read2)

		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		engine.Run()
	})

	It("should do read hit", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.ReadReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x100).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Deliver(read1)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		engine.Run()
		t1 := engine.CurrentTime()

		read2 := mem.ReadReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Deliver(read2)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})
		engine.Run()
		t2 := engine.CurrentTime()

		Expect(t2 - t1).To(BeNumerically("<", t1))
	})

	It("should write partial line", func() {
		write := mem.WriteReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x100).
			WithData([]byte{1, 2, 3, 4}).
			Build()
		c.GetPortByName("Top").Deliver(write)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should write full line", func() {
		write := mem.WriteReqBuilder{}.
			WithSrc(cuPort.AsRemote()).
			WithDst(c.GetPortByName("Top").AsRemote()).
			WithAddress(0x100).
			WithData(
				[]byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				}).
			Build()
		c.GetPortByName("Top").Deliver(write)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})
		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

})
