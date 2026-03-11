package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("TopParser", func() {
	var (
		mockCtrl            *gomock.Controller
		m                   *middleware
		parser              *topParser
		port                *MockPort
		buf                 *MockBuffer
		addressToPortMapper *MockAddressToPortMapper
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		port = NewMockPort(mockCtrl)
		buf = NewMockBuffer(mockCtrl)

		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		comp := MakeBuilder().
			WithEngine(sim.NewSerialEngine()).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 2, 2, "Cache.ToTop")).
			WithBottomPort(sim.NewPort(nil, 2, 2, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 2, 2, "Cache.ControlPort")).
			Build("Cache")

		m = comp.Middlewares()[0].(*middleware)

		parser = &topParser{
			cache: m,
		}
		m.state = cacheStateRunning
		m.topPort = port
		m.dirStageBuffer = buf
		m.inFlightTransactions = nil
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should return if no req to parse", func() {
		port.EXPECT().PeekIncoming().Return(nil)
		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should return if the cache is not in running stage", func() {
		m.state = cacheStateFlushing
		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should return if the dir buf is full", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x100
		read.AccessByteSize = 64
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		port.EXPECT().PeekIncoming().Return(read)
		buf.EXPECT().CanPush().Return(false)

		ret := parser.Tick()

		Expect(ret).To(BeFalse())
	})

	It("should parse read from top", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x100
		read.AccessByteSize = 64
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		port.EXPECT().PeekIncoming().Return(read)
		buf.EXPECT().CanPush().Return(true)
		buf.EXPECT().Push(gomock.Any()).Do(func(t *transactionState) {
			Expect(t.read).To(BeIdenticalTo(read))
		})
		port.EXPECT().RetrieveIncoming().Return(read)

		parser.Tick()

		Expect(m.inFlightTransactions).To(HaveLen(1))
	})

	It("should parse write from top", func() {
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Address = 0x100
		write.TrafficBytes = 12
		write.TrafficClass = "mem.WriteReq"

		port.EXPECT().PeekIncoming().Return(write)
		buf.EXPECT().CanPush().Return(true)
		buf.EXPECT().Push(gomock.Any()).Do(func(t *transactionState) {
			Expect(t.write).To(BeIdenticalTo(write))
		})
		port.EXPECT().RetrieveIncoming().Return(write)

		parser.Tick()

		Expect(m.inFlightTransactions).To(HaveLen(1))
	})

})
