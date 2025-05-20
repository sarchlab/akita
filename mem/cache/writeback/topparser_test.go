package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("TopParser", func() {
	var (
		mockCtrl            *gomock.Controller
		cache               *Comp
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

		builder := MakeBuilder().
			WithAddressToPortMapper(addressToPortMapper)
		cache = builder.Build("Cache")

		parser = &topParser{
			cache: cache,
		}
		cache.state = cacheStateRunning
		cache.topPort = port
		cache.dirStageBuffer = buf
		cache.inFlightTransactions = nil
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
		cache.state = cacheStateFlushing
		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should return if the dir buf is full", func() {
		read := mem.ReadReqBuilder{}.
			WithAddress(0x100).
			WithByteSize(64).
			Build()
		port.EXPECT().PeekIncoming().Return(read)
		buf.EXPECT().CanPush().Return(false)

		ret := parser.Tick()

		Expect(ret).To(BeFalse())
	})

	It("should parse read from top", func() {
		read := mem.ReadReqBuilder{}.
			WithAddress(0x100).
			WithByteSize(64).
			Build()

		port.EXPECT().PeekIncoming().Return(read)
		buf.EXPECT().CanPush().Return(true)
		buf.EXPECT().Push(gomock.Any()).Do(func(t *transaction) {
			Expect(t.read).To(BeIdenticalTo(read))
		})
		port.EXPECT().RetrieveIncoming().Return(read)

		parser.Tick()

		Expect(cache.inFlightTransactions).To(HaveLen(1))
	})

	It("should parse write from top", func() {
		write := mem.WriteReqBuilder{}.
			WithAddress(0x100).
			Build()

		port.EXPECT().PeekIncoming().Return(write)
		buf.EXPECT().CanPush().Return(true)
		buf.EXPECT().Push(gomock.Any()).Do(func(t *transaction) {
			Expect(t.write).To(BeIdenticalTo(write))
		})
		port.EXPECT().RetrieveIncoming().Return(write)

		parser.Tick()

		Expect(cache.inFlightTransactions).To(HaveLen(1))
	})

})
