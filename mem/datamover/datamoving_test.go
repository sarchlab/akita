package datamover

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

var _ = Describe("DataMover", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     sim.Engine
		dataMover  *Comp
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

		insideMem = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithCacheLineSize(64).
			WithNewStorage(1 * mem.MB).
			Build("InsideMem")
		outsideMem = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithCacheLineSize(64).
			WithNewStorage(1 * mem.MB).
			Build("OutsideMem")
		dataMover = MakeBuilder().
			WithEngine(engine).
			WithBufferSize(2048).
			WithInsidePortMapper(&mem.SinglePortMapper{
				Port: insideMem.GetPortByName("Top"),
			}).
			WithOutsidePortMapper(&mem.SinglePortMapper{
				Port: outsideMem.GetPortByName("Top"),
			}).
			WithInsideByteGranularity(64).
			WithOutsideByteGranularity(256).
			Build("DataMover")

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		conn.PlugIn(srcPort, 64)
		conn.PlugIn(dataMover.ctrlPort, 64)
		conn.PlugIn(dataMover.insidePort, 64)
		conn.PlugIn(dataMover.outsidePort, 64)
		conn.PlugIn(insideMem.GetPortByName("Top"), 64)
		conn.PlugIn(outsideMem.GetPortByName("Top"), 64)
	})

	FIt("should move data", func() {
		data := make([]byte, 4096)
		for i := 0; i < 4096; i++ {
			data[i] = byte(i)
		}
		outsideMem.Storage.Write(0, data)

		srcPort.EXPECT().
			Deliver(gomock.AssignableToTypeOf(&sim.GeneralRsp{}))

		req := MakeDataMoveRequestBuilder().
			WithSrc(srcPort).
			WithDst(dataMover.ctrlPort).
			WithSrcAddress(0).
			WithSrcSide("outside").
			WithDstAddress(0).
			WithDstSide("inside").
			WithByteSize(4096).
			Build()

		dataMover.ctrlPort.Deliver(req)

		engine.Run()

		Expect(insideMem.Storage.Read(0, 4096)).To(Equal(data))
	})
})

// var _ = Describe("Datamoving", func() {
// 	var (
// 		mockCtrl   *gomock.Controller
// 		engine     *MockEngine
// 		SrcPort    *MockPort
// 		DstPort    *MockPort
// 		CtrlPort   *MockPort
// 		portMapper *mem.SinglePortMapper
// 		sdmEngine  *Comp
// 	)

// 	BeforeEach(func() {
// 		mockCtrl = gomock.NewController(GinkgoT())
// 		engine = NewMockEngine(mockCtrl)
// 		SrcPort = NewMockPort(mockCtrl)
// 		DstPort = NewMockPort(mockCtrl)
// 		CtrlPort = NewMockPort(mockCtrl)

// 		portMapper = new(mem.SinglePortMapper)
// 		sdmEngine = new(Builder).
// 			WithName("SDMTest").
// 			WithEngine(engine).
// 			WithInsideByteGranularity(64).
// 			WithOutsideByteGranularity(256).
// 			WithInsidePortMapper(portMapper).
// 			WithBufferSize(2048).
// 			Build()
// 		sdmEngine.insidePort = SrcPort
// 		sdmEngine.outsidePort = DstPort
// 		sdmEngine.ctrlPort = CtrlPort
// 	})

// 	AfterEach(func() {
// 		mockCtrl.Finish()
// 	})

// 	It("should not operate when there are no requests", func() {
// 		CtrlPort.EXPECT().RetrieveIncoming().Return(nil)
// 		madeProgress := sdmEngine.parseFromCP()
// 		Expect(madeProgress).To(BeFalse())
// 	})

// 	It("should parse DataReady from srcPort", func() {
// 		dmBuilder := MakeDataMoveRequestBuilder().
// 			WithByteSize(200).
// 			WithSrcPort(InsidePort).
// 			WithSrcAddress(20).
// 			WithDstAddress(40).
// 			WithDst(CtrlPort)
// 		dmRequest := dmBuilder.Build()
// 		rqC := NewRequestCollection(dmRequest)
// 		sdmEngine.currentTransaction = rqC

// 		readReq1 := mem.ReadReqBuilder{}.
// 			WithSrc(SrcPort).
// 			WithAddress(20).
// 			WithByteSize(64).
// 			Build()
// 		readReq2 := mem.ReadReqBuilder{}.
// 			WithSrc(SrcPort).
// 			WithAddress(84).
// 			WithByteSize(64).
// 			Build()
// 		readReq3 := mem.ReadReqBuilder{}.
// 			WithSrc(SrcPort).
// 			WithAddress(148).
// 			WithByteSize(64).
// 			Build()
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq1)
// 		rqC.appendSubReq(readReq1.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq2)
// 		rqC.appendSubReq(readReq2.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq3)
// 		rqC.appendSubReq(readReq3.Meta().ID)

// 		dataReady := mem.DataReadyRspBuilder{}.
// 			WithDst(SrcPort).
// 			WithRspTo(readReq1.Meta().ID).
// 			WithData([]byte{
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 			}).Build()
// 		SrcPort.EXPECT().RetrieveIncoming().Return(dataReady)

// 		madeProgress := sdmEngine.parseFromSrc()

// 		Expect(madeProgress).To(BeTrue())
// 		Expect(sdmEngine.currentTransaction.req).To(BeIdenticalTo(dmRequest))
// 		Expect(sdmEngine.currentTransaction).To(BeIdenticalTo(rqC))
// 		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(readReq1))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq2))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq3))
// 		Expect(sdmEngine.buffer[0:64]).To(Equal(dataReady.Data))
// 	})

// 	It("should parse DataReady from dstPort", func() {
// 		dmBuilder := new(DataMoveRequestBuilder).
// 			WithByteSize(200).
// 			WithSrcPort(OutsidePort).
// 			WithDstPort(InsidePort).
// 			WithSrcTransferSize(256).
// 			WithDstTransferSize(64).
// 			WithSrcAddress(40).
// 			WithDstAddress(20).
// 			WithDst(CtrlPort)
// 		dmRequest := dmBuilder.Build()
// 		rqC := NewRequestCollection(dmRequest)
// 		sdmEngine.currentTransaction = rqC

// 		readReq1 := mem.ReadReqBuilder{}.
// 			WithSrc(DstPort).
// 			WithAddress(20).
// 			WithByteSize(64).
// 			Build()
// 		readReq2 := mem.ReadReqBuilder{}.
// 			WithSrc(DstPort).
// 			WithAddress(84).
// 			WithByteSize(64).
// 			Build()
// 		readReq3 := mem.ReadReqBuilder{}.
// 			WithSrc(DstPort).
// 			WithAddress(148).
// 			WithByteSize(64).
// 			Build()
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq1)
// 		rqC.appendSubReq(readReq1.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq2)
// 		rqC.appendSubReq(readReq2.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, readReq3)
// 		rqC.appendSubReq(readReq3.Meta().ID)

// 		dataReady := mem.DataReadyRspBuilder{}.
// 			WithDst(DstPort).
// 			WithRspTo(readReq1.Meta().ID).
// 			WithData([]byte{
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 				1, 2, 3, 4, 5, 6, 7, 8,
// 			}).Build()
// 		DstPort.EXPECT().RetrieveIncoming().Return(dataReady)

// 		madeProgress := sdmEngine.parseFromDst()

// 		Expect(madeProgress).To(BeTrue())
// 		Expect(sdmEngine.currentTransaction.req).To(BeIdenticalTo(dmRequest))
// 		Expect(sdmEngine.currentTransaction).To(BeIdenticalTo(rqC))
// 		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(readReq1))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq2))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(readReq3))
// 		Expect(sdmEngine.buffer[0:64]).To(Equal(dataReady.Data))
// 	})

// 	It("should parse WriteDone from srcPort", func() {
// 		dmBuilder := new(DataMoveRequestBuilder).
// 			WithByteSize(200).
// 			WithSrcPort(OutsidePort).
// 			WithDstPort(InsidePort).
// 			WithSrcTransferSize(64).
// 			WithDstTransferSize(64).
// 			WithSrcAddress(40).
// 			WithDstAddress(20).
// 			WithDst(CtrlPort)
// 		dmRequest := dmBuilder.Build()
// 		rqC := NewRequestCollection(dmRequest)
// 		sdmEngine.currentTransaction = rqC

// 		writeReq1 := mem.WriteReqBuilder{}.
// 			WithSrc(SrcPort).
// 			WithAddress(40).
// 			Build()
// 		writeReq2 := mem.WriteReqBuilder{}.
// 			WithSrc(SrcPort).
// 			WithAddress(104).
// 			Build()
// 		writeReq3 := mem.WriteReqBuilder{}.
// 			WithSrc(SrcPort).
// 			WithAddress(168).
// 			Build()
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq1)
// 		rqC.appendSubReq(writeReq1.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq2)
// 		rqC.appendSubReq(writeReq2.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq3)
// 		rqC.appendSubReq(writeReq3.Meta().ID)

// 		writeDone := mem.WriteDoneRspBuilder{}.
// 			WithDst(SrcPort).
// 			WithRspTo(writeReq1.Meta().ID).
// 			Build()
// 		SrcPort.EXPECT().RetrieveIncoming().Return(writeDone)

// 		madeProgress := sdmEngine.parseFromSrc()

// 		Expect(madeProgress).To(BeTrue())
// 		Expect(sdmEngine.currentTransaction.req).To(BeIdenticalTo(dmRequest))
// 		Expect(sdmEngine.currentTransaction).To(BeIdenticalTo(rqC))
// 		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(writeReq1))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq2))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq3))
// 	})

// 	It("should parse WriteDone from dstPort", func() {
// 		dmBuilder := MakeDataMoveRequestBuilder().
// 			WithByteSize(200).
// 			WithSrcPort(InsidePort).
// 			WithDstPort(OutsidePort).
// 			WithSrcTransferSize(64).
// 			WithDstTransferSize(64).
// 			WithSrcAddress(20).
// 			WithDstAddress(40).
// 			WithDst(CtrlPort)
// 		dmRequest := dmBuilder.Build()
// 		rqC := NewRequestCollection(dmRequest)
// 		sdmEngine.currentTransaction = rqC

// 		writeReq1 := mem.WriteReqBuilder{}.
// 			WithSrc(DstPort).
// 			WithAddress(40).
// 			Build()
// 		writeReq2 := mem.WriteReqBuilder{}.
// 			WithSrc(DstPort).
// 			WithAddress(104).
// 			Build()
// 		writeReq3 := mem.WriteReqBuilder{}.
// 			WithSrc(DstPort).
// 			WithAddress(168).
// 			Build()
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq1)
// 		rqC.appendSubReq(writeReq1.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq2)
// 		rqC.appendSubReq(writeReq2.Meta().ID)
// 		sdmEngine.pendingRequests = append(sdmEngine.pendingRequests, writeReq3)
// 		rqC.appendSubReq(writeReq3.Meta().ID)

// 		writeDone := mem.WriteDoneRspBuilder{}.
// 			WithDst(DstPort).
// 			WithRspTo(writeReq1.Meta().ID).
// 			Build()
// 		DstPort.EXPECT().RetrieveIncoming().Return(writeDone)

// 		madeProgress := sdmEngine.parseFromDst()

// 		Expect(madeProgress).To(BeTrue())
// 		Expect(sdmEngine.currentTransaction.req).To(BeIdenticalTo(dmRequest))
// 		Expect(sdmEngine.currentTransaction).To(BeIdenticalTo(rqC))
// 		Expect(sdmEngine.pendingRequests).NotTo(ContainElement(writeReq1))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq2))
// 		Expect(sdmEngine.pendingRequests).To(ContainElement(writeReq3))
// 	})

// 	It("should parse dmRequest from CP", func() {
// 		dmBuilder := MakeDataMoveRequestBuilder().
// 			WithByteSize(200).
// 			WithSrcTransferSize(64).
// 			WithDstTransferSize(256).
// 			WithSrcAddress(20).
// 			WithDstAddress(40).
// 			WithDst(CtrlPort)
// 		dmRequest := dmBuilder.Build()
// 		CtrlPort.EXPECT().RetrieveIncoming().Return(dmRequest)
// 		SrcPort.EXPECT().Send(gomock.Any()).Return(nil).AnyTimes()
// 		DstPort.EXPECT().Send(gomock.Any()).Return(nil).AnyTimes()

// 		madeProgress := sdmEngine.parseFromCP()

// 		Expect(madeProgress).To(BeTrue())
// 		Expect(sdmEngine.currentTransaction.req).To(BeIdenticalTo(dmRequest))
// 		Expect(sdmEngine.pendingRequests).To(HaveLen(5))
// 		Expect(sdmEngine.pendingRequests[0].(*mem.ReadReq).Address).
// 			To(Equal(uint64(20)))
// 		Expect(sdmEngine.pendingRequests[1].(*mem.ReadReq).Address).
// 			To(Equal(uint64(84)))
// 		Expect(sdmEngine.pendingRequests[2].(*mem.ReadReq).Address).
// 			To(Equal(uint64(148)))
// 		Expect(sdmEngine.pendingRequests[3].(*mem.ReadReq).Address).
// 			To(Equal(uint64(212)))
// 		Expect(sdmEngine.pendingRequests[4].(*mem.WriteReq).Address).
// 			To(Equal(uint64(40)))
// 	})
// })
