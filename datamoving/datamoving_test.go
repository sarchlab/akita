package datamoving

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/mem/mem"
)

var _ = Describe("Datamoving", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		SrcPort           *MockPort
		DstPort           *MockPort
		CtrlPort          *MockPort
		localModuleFinder *mem.SingleLowModuleFinder
		sdmEngine         *StreamingDataMover
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		SrcPort = NewMockPort(mockCtrl)
		DstPort = NewMockPort(mockCtrl)
		CtrlPort = NewMockPort(mockCtrl)

		localModuleFinder = new(mem.SingleLowModuleFinder)
		sdmEngine = NewSDMEngine("SDM", engine, localModuleFinder)
		sdmEngine.SrcPort = SrcPort
		sdmEngine.DstPort = DstPort
		sdmEngine.CtrlPort = CtrlPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should not exceed the maximum request count", func() {
		for i := 0; i < sdmEngine.GetMaxRequestCount(); i++ {
			srcBuffer := make([]byte, 128)
			dstBuffer := make([]byte, 128)
			reqBuilder := DataMoveRequestBuilder{}
			// Builder method needs to complete
			req := reqBuilder.Build()
			rqC := NewRequestCollection(req)

			sdmEngine.processingRequests = append(sdmEngine.processingRequests, rqC)
		}
		madeProgress := sdmEngine.parseFromCP()

		Expect((sdmEngine.toSrc)).To(HaveLen(0))
		Expect((madeProgress)).To(BeFalse())
	})

	It("should parse dmRequest from CP", func() {
		// TODO
	})

	It("should parse DataReady from srcPort", func() {
		// TODO
	})

	It("should respond to DataReady from srcPort", func() {
		// TODO
	})

	It("should parse WriteDone from srcPort", func() {
		// TODO
	})

	It("should respond to WriteDone from srcPort", func() {
		// TODO
	})

	It("should parse DataReady from dstPort", func() {
		// TODO
	})

	It("should respond to DataReady from dstPort", func() {
		// TODO
	})

	It("should parse WriteDone from dstPort", func() {
		// TODO
	})

	It("should respond to WriteDone from dstPort", func() {
		// TODO
	})

})
