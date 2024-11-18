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
		sdmBuilder := new(Builder)
		sdmBuilder.WithName("SDMTest")
		sdmBuilder.WithEngine(engine)
		sdmBuilder.WithLocalDataSource(localModuleFinder)
		sdmEngine = sdmBuilder.Build()
		sdmEngine.SrcPort = SrcPort
		sdmEngine.DstPort = DstPort
		sdmEngine.CtrlPort = CtrlPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should not operate when there are no requests", func() {
		CtrlPort.EXPECT().RetrieveIncoming().Return(nil)
		madeProgress := sdmEngine.parseFromCP()
		Expect(madeProgress).To(BeFalse())
	})

	It("should parse dmRequest from CP", func() {
		dmBuilder := new(DataMoveRequestBuilder)
		dmBuilder.WithByteSize(200)
		dmBuilder.WithDirection("s2d")
		dmBuilder.WithSrcTransferSize(64)
		dmBuilder.WithDstTransferSize(256)
		dmBuilder.WithSrcAddress(20)
		dmBuilder.WithDstAddress(40)
		dmBuilder.WithDst(CtrlPort)
		dmRequest := dmBuilder.Build()
		CtrlPort.EXPECT().RetrieveIncoming().Return(dmRequest)

		madeProgress := sdmEngine.parseFromCP()

		Expect(madeProgress).To(BeTrue())
		Expect(sdmEngine.currentRequest.topReq).To(BeIdenticalTo(dmRequest))
		Expect(sdmEngine.toSrc).To(HaveLen(4))
		Expect(sdmEngine.toSrc[0].(*mem.ReadReq).Address).
			To(Equal(uint64(20)))
		Expect(sdmEngine.toSrc[1].(*mem.ReadReq).Address).
			To(Equal(uint64(84)))
		Expect(sdmEngine.toSrc[2].(*mem.ReadReq).Address).
			To(Equal(uint64(148)))
		Expect(sdmEngine.toSrc[3].(*mem.ReadReq).Address).
			To(Equal(uint64(212)))
		Expect(sdmEngine.toDst).To(HaveLen(1))
		Expect(sdmEngine.toDst[0].(*mem.WriteReq).Address).
			To(Equal(uint64(40)))
	})

	It("should parse DataReady from srcPort", func() {
		dmBuilder := new(DataMoveRequestBuilder)
		dmBuilder.WithByteSize(200)
		dmBuilder.WithDirection("s2d")
		dmBuilder.WithSrcTransferSize(64)
		dmBuilder.WithDstTransferSize(256)
		dmBuilder.WithSrcAddress(20)
		dmBuilder.WithDstAddress(40)
		dmBuilder.WithDst(CtrlPort)
		dmRequest := dmBuilder.Build()
		rqC := NewRequestCollection(dmRequest)
		sdmEngine.currentRequest = rqC

		readReq1 := mem.ReadReqBuilder{}.
			WithSrc(SrcPort).
			WithAddress(20).
			WithByteSize(64).
			Build()
		readReq2 := mem.ReadReqBuilder{}.
			WithSrc(SrcPort).
			WithAddress(84).
			WithByteSize(64).
			Build()
		readReq3 := mem.ReadReqBuilder{}.
			WithSrc(SrcPort).
			WithAddress(148).
			WithByteSize(64).
			Build()
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq1)
		rqC.appendSubReq(readReq1.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq2)
		rqC.appendSubReq(readReq2.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq3)
		rqC.appendSubReq(readReq3.Meta().ID)

		dataReady := mem.DataReadyRspBuilder{}.
			WithDst(SrcPort).
			WithRspTo(readReq1.Meta().ID).
			WithData([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}).Build()
		SrcPort.EXPECT().RetrieveIncoming().Return(dataReady)

		madeProgress := sdmEngine.parseFromSrc()

		Expect(madeProgress).To(BeTrue())
		Expect(sdmEngine.currentRequest.topReq).To(BeIdenticalTo(dmRequest))
		Expect(sdmEngine.currentRequest).To(BeIdenticalTo(rqC))
		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(readReq1))
		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq2))
		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq3))
		Expect(sdmEngine.buffer[0:64]).To(Equal(dataReady.Data))
	})

	It("should parse WriteDone from srcPort", func() {
		dmBuilder := new(DataMoveRequestBuilder)
		dmBuilder.WithByteSize(200)
		dmBuilder.WithDirection("d2s")
		dmBuilder.WithSrcTransferSize(64)
		dmBuilder.WithDstTransferSize(64)
		dmBuilder.WithSrcAddress(40)
		dmBuilder.WithDstAddress(20)
		dmBuilder.WithDst(CtrlPort)
		dmRequest := dmBuilder.Build()
		rqC := NewRequestCollection(dmRequest)
		sdmEngine.currentRequest = rqC

		writeReq1 := mem.WriteReqBuilder{}.
			WithSrc(SrcPort).
			WithAddress(40).
			Build()
		writeReq2 := mem.WriteReqBuilder{}.
			WithSrc(SrcPort).
			WithAddress(104).
			Build()
		writeReq3 := mem.WriteReqBuilder{}.
			WithSrc(SrcPort).
			WithAddress(168).
			Build()
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq1)
		rqC.appendSubReq(writeReq1.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq2)
		rqC.appendSubReq(writeReq2.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq3)
		rqC.appendSubReq(writeReq3.Meta().ID)

		writeDone := mem.WriteDoneRspBuilder{}.
			WithDst(SrcPort).
			WithRspTo(writeReq1.Meta().ID).
			Build()
		SrcPort.EXPECT().RetrieveIncoming().Return(writeDone)

		madeProgress := SrcPort.RetrieveIncoming()

		Expect(madeProgress).To(BeTrue())
		Expect(sdmEngine.currentRequest.topReq).To(BeIdenticalTo(dmRequest))
		Expect(sdmEngine.currentRequest).To(BeIdenticalTo(rqC))
		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(writeReq1))
		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq2))
		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq3))
	})

	It("should parse DataReady from dstPort", func() {
		dmBuilder := new(DataMoveRequestBuilder)
		dmBuilder.WithByteSize(200)
		dmBuilder.WithDirection("d2s")
		dmBuilder.WithSrcTransferSize(256)
		dmBuilder.WithDstTransferSize(64)
		dmBuilder.WithSrcAddress(40)
		dmBuilder.WithDstAddress(20)
		dmBuilder.WithDst(CtrlPort)
		dmRequest := dmBuilder.Build()
		rqC := NewRequestCollection(dmRequest)
		sdmEngine.currentRequest = rqC

		readReq1 := mem.ReadReqBuilder{}.
			WithSrc(DstPort).
			WithAddress(20).
			WithByteSize(64).
			Build()
		readReq2 := mem.ReadReqBuilder{}.
			WithSrc(DstPort).
			WithAddress(84).
			WithByteSize(64).
			Build()
		readReq3 := mem.ReadReqBuilder{}.
			WithSrc(DstPort).
			WithAddress(148).
			WithByteSize(64).
			Build()
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq1)
		rqC.appendSubReq(readReq1.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq2)
		rqC.appendSubReq(readReq2.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq3)
		rqC.appendSubReq(readReq3.Meta().ID)

		dataReady := mem.DataReadyRspBuilder{}.
			WithDst(DstPort).
			WithRspTo(readReq1.Meta().ID).
			WithData([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}).Build()
		DstPort.EXPECT().RetrieveIncoming().Return(dataReady)

		madeProgress := sdmEngine.parseFromDst()

		Expect(madeProgress).To(BeTrue())
		Expect(sdmEngine.currentRequest.topReq).To(BeIdenticalTo(dmRequest))
		Expect(sdmEngine.currentRequest).To(BeIdenticalTo(rqC))
		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(readReq1))
		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq2))
		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq3))
		Expect(sdmEngine.buffer[0:64]).To(Equal(dataReady.Data))
	})

	It("should parse WriteDone from dstPort", func() {
		dmBuilder := new(DataMoveRequestBuilder)
		dmBuilder.WithByteSize(200)
		dmBuilder.WithDirection("s2d")
		dmBuilder.WithSrcTransferSize(64)
		dmBuilder.WithDstTransferSize(64)
		dmBuilder.WithSrcAddress(20)
		dmBuilder.WithDstAddress(40)
		dmBuilder.WithDst(CtrlPort)
		dmRequest := dmBuilder.Build()
		rqC := NewRequestCollection(dmRequest)
		sdmEngine.currentRequest = rqC

		writeReq1 := mem.WriteReqBuilder{}.
			WithSrc(DstPort).
			WithAddress(40).
			Build()
		writeReq2 := mem.WriteReqBuilder{}.
			WithSrc(DstPort).
			WithAddress(104).
			Build()
		writeReq3 := mem.WriteReqBuilder{}.
			WithSrc(DstPort).
			WithAddress(168).
			Build()
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq1)
		rqC.appendSubReq(writeReq1.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq2)
		rqC.appendSubReq(writeReq2.Meta().ID)
		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq3)
		rqC.appendSubReq(writeReq3.Meta().ID)

		writeDone := mem.WriteDoneRspBuilder{}.
			WithDst(DstPort).
			WithRspTo(writeReq1.Meta().ID).
			Build()
		DstPort.EXPECT().RetrieveIncoming().Return(writeDone)

		madeProgress := DstPort.RetrieveIncoming()

		Expect(madeProgress).To(BeTrue())
		Expect(sdmEngine.currentRequest.topReq).To(BeIdenticalTo(dmRequest))
		Expect(sdmEngine.currentRequest).To(BeIdenticalTo(rqC))
		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(writeReq1))
		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq2))
		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq3))
	})
})
