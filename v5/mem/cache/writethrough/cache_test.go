package writethrough_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"

	. "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"

	"github.com/sarchlab/akita/v5/mem/mem"
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
			WithTopPort(sim.NewPort(nil, 16, 16, "DRAM.TopPort")).
			WithCtrlPort(sim.NewPort(nil, 16, 16, "DRAM.CtrlPort")).
			Build("DRAM")
		addressToPortMapper = &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}
		c = MakeBuilder().
			WithEngine(engine).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 4, 4, "Cache.TopPort")).
			WithBottomPort(sim.NewPort(nil, 4, 4, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 4, 4, "Cache.ControlPort")).
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
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = cuPort.AsRemote()
		read.Dst = c.GetPortByName("Top").AsRemote()
		read.Address = 0x100
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		c.GetPortByName("Top").Deliver(read)

		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})

		engine.Run()
	})

	It("should do read miss coalesce", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := &mem.ReadReq{}
		read1.ID = sim.GetIDGenerator().Generate()
		read1.Src = cuPort.AsRemote()
		read1.Dst = c.GetPortByName("Top").AsRemote()
		read1.Address = 0x100
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "mem.ReadReq"
		c.GetPortByName("Top").Deliver(read1)

		read2 := &mem.ReadReq{}
		read2.ID = sim.GetIDGenerator().Generate()
		read2.Src = cuPort.AsRemote()
		read2.Dst = c.GetPortByName("Top").AsRemote()
		read2.Address = 0x104
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "mem.ReadReq"
		c.GetPortByName("Top").Deliver(read2)

		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		engine.Run()
	})

	It("should do read hit", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := &mem.ReadReq{}
		read1.ID = sim.GetIDGenerator().Generate()
		read1.Src = cuPort.AsRemote()
		read1.Dst = c.GetPortByName("Top").AsRemote()
		read1.Address = 0x100
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "mem.ReadReq"
		c.GetPortByName("Top").Deliver(read1)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		engine.Run()
		t1 := engine.CurrentTime()

		read2 := &mem.ReadReq{}
		read2.ID = sim.GetIDGenerator().Generate()
		read2.Src = cuPort.AsRemote()
		read2.Dst = c.GetPortByName("Top").AsRemote()
		read2.Address = 0x104
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "mem.ReadReq"
		c.GetPortByName("Top").Deliver(read2)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})
		engine.Run()
		t2 := engine.CurrentTime()

		Expect(t2 - t1).To(BeNumerically("<", t1))
	})

	It("should write partial line", func() {
		writeData := []byte{1, 2, 3, 4}
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Src = cuPort.AsRemote()
		write.Dst = c.GetPortByName("Top").AsRemote()
		write.Address = 0x100
		write.Data = writeData
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "mem.WriteReq"
		c.GetPortByName("Top").Deliver(write)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(write.ID))
			})

		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should write full line", func() {
		writeData2 := []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Src = cuPort.AsRemote()
		write.Dst = c.GetPortByName("Top").AsRemote()
		write.Address = 0x100
		write.Data = writeData2
		write.TrafficBytes = len(writeData2) + 12
		write.TrafficClass = "mem.WriteReq"
		c.GetPortByName("Top").Deliver(write)
		cuPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(write.ID))
			})
		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

})
