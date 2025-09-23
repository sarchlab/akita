package addresstranslator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Address Translator", func() {
	var (
		mockCtrl              *gomock.Controller
		topPort               *MockPort
		bottomPort            *MockPort
		translationPort       *MockPort
		ctrlPort              *MockPort
		memoryPortMapper      *MockAddressToPortMapper
		translationPortMapper *MockAddressToPortMapper

		t           *Comp
		tMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		topPort.EXPECT().
			Name().
			Return("TopPort").
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()
		bottomPort.EXPECT().
			Name().
			Return("BottomPort").
			AnyTimes()
		ctrlPort = NewMockPort(mockCtrl)
		ctrlPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("CtrlPort")).
			AnyTimes()
		translationPort = NewMockPort(mockCtrl)
		translationPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TranslationPort")).
			AnyTimes()
		translationPort.EXPECT().
			Name().
			Return("TranslationPort").
			AnyTimes()
		memoryPortMapper = NewMockAddressToPortMapper(mockCtrl)
		translationPortMapper = NewMockAddressToPortMapper(mockCtrl)

		builder := MakeBuilder().
			WithLog2PageSize(12).
			WithFreq(1).
			WithMemoryProviderMapper(memoryPortMapper).
			WithTranslationProviderMapper(translationPortMapper)

		t = builder.Build("AddressTranslator")
		t.log2PageSize = 12
		t.topPort = topPort
		t.bottomPort = bottomPort
		t.translationPort = translationPort
		t.ctrlPort = ctrlPort

		tMiddleware = t.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("translate stage", func() {
		var (
			req *mem.ReadReq
		)

		BeforeEach(func() {
			req = mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithByteSize(4).
				WithPID(1).
				Build()
		})

		It("should do nothing if there is no request", func() {
			topPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tMiddleware.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			var transReqReturn *vm.TranslationReq
			transReq := vm.TranslationReqBuilder{}.
				WithPID(1).
				WithVAddr(0x100).
				WithDeviceID(1).
				Build()

			translation := &transaction{
				translationReq: transReq,
			}
			t.transactions = append(t.transactions, translation)
			req.Address = 0x1040

			translationPortMapper.EXPECT().
				Find(uint64(0x1040)).
				Return(translationPort.AsRemote())

			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().RetrieveIncoming()
			translationPort.EXPECT().Send(gomock.Any()).
				DoAndReturn(func(req *vm.TranslationReq) *sim.SendError {
					transReqReturn = req
					return nil
				})

			needTick := tMiddleware.translate()

			Expect(needTick).To(BeTrue())
			Expect(translation.incomingReqs).NotTo(ContainElement(req))
			Expect(t.transactions).To(HaveLen(2))
			Expect(t.transactions[1].translationReq).
				To(BeEquivalentTo(transReqReturn))
		})

		It("should stall if cannot send for translation", func() {
			translationPortMapper.EXPECT().
				Find(uint64(0x100)).
				Return(translationPort.AsRemote())
			topPort.EXPECT().PeekIncoming().Return(req)
			translationPort.EXPECT().
				Send(gomock.Any()).
				Return(&sim.SendError{})

			needTick := tMiddleware.translate()

			Expect(needTick).To(BeFalse())
			Expect(t.transactions).To(HaveLen(0))
		})
	})

	Context("parse translation", func() {
		var (
			transReq1, transReq2 *vm.TranslationReq
			trans1, trans2       *transaction
		)

		BeforeEach(func() {
			transReq1 = vm.TranslationReqBuilder{}.
				WithPID(1).
				WithVAddr(0x100).
				WithDeviceID(1).
				Build()
			trans1 = &transaction{
				translationReq: transReq1,
			}
			transReq2 = vm.TranslationReqBuilder{}.
				WithPID(1).
				WithVAddr(0x100).
				WithDeviceID(1).
				Build()
			trans2 = &transaction{
				translationReq: transReq2,
			}
			t.transactions = append(t.transactions, trans1, trans2)
		})

		It("should do nothing if there is no translation return", func() {
			translationPort.EXPECT().PeekIncoming().Return(nil)
			needTick := tMiddleware.parseTranslation()
			Expect(needTick).To(BeFalse())
		})

		It("should stall if send failed", func() {
			req := mem.ReadReqBuilder{}.
				WithAddress(0x10040).
				WithByteSize(4).
				Build()
			translationRsp := vm.TranslationRspBuilder{}.
				WithRspTo(transReq1.ID).
				WithPage(vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				}).
				Build()

			trans1.incomingReqs = []mem.AccessReq{req}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			memoryPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).Return(sim.NewSendError())

			madeProgress := tMiddleware.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := mem.ReadReqBuilder{}.
				WithAddress(0x10040).
				WithByteSize(4).
				Build()
			translationRsp := vm.TranslationRspBuilder{}.
				WithRspTo(transReq1.ID).
				WithPage(vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				}).
				Build()

			trans1.incomingReqs = []mem.AccessReq{req}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			memoryPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(read *mem.ReadReq) {
					Expect(read).NotTo(BeIdenticalTo(req))
					Expect(read.PID).To(Equal(vm.PID(0)))
					Expect(read.Address).To(Equal(uint64(0x20040)))
					Expect(read.AccessByteSize).To(Equal(uint64(4)))
					Expect(read.Src).To(Equal(bottomPort.AsRemote()))
				}).
				Return(nil)

			madeProgress := tMiddleware.parseTranslation()

			Expect(madeProgress).To(BeTrue())
			Expect(t.transactions).NotTo(ContainElement(trans1))
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})

		It("should forward write request", func() {
			data := []byte{1, 2, 3, 4}
			dirty := []bool{false, true, false, true}
			write := mem.WriteReqBuilder{}.
				WithAddress(0x10040).
				WithData(data).
				WithDirtyMask(dirty).
				Build()
			translationRsp := vm.TranslationRspBuilder{}.
				WithRspTo(transReq1.ID).
				WithPage(vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				}).
				Build()
			trans1.incomingReqs = []mem.AccessReq{write}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			memoryPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(req *mem.WriteReq) {
					Expect(req).NotTo(BeIdenticalTo(write))
					Expect(req.PID).To(Equal(vm.PID(0)))
					Expect(req.Address).To(Equal(uint64(0x20040)))
					Expect(req.Src).To(Equal(bottomPort.AsRemote()))
					Expect(req.Data).To(Equal(data))
					Expect(req.DirtyMask).To(Equal(dirty))
				}).
				Return(nil)

			madeProgress := tMiddleware.parseTranslation()

			Expect(madeProgress).To(BeTrue())
			Expect(t.transactions).NotTo(ContainElement(trans1))
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})
	})

	Context("respond", func() {
		var (
			readFromTop   *mem.ReadReq
			writeFromTop  *mem.WriteReq
			readToBottom  *mem.ReadReq
			writeToBottom *mem.WriteReq
		)

		BeforeEach(func() {
			readFromTop = mem.ReadReqBuilder{}.
				WithAddress(0x10040).
				WithByteSize(4).
				Build()
			readToBottom = mem.ReadReqBuilder{}.
				WithAddress(0x20040).
				WithByteSize(4).
				Build()
			writeFromTop = mem.WriteReqBuilder{}.
				WithAddress(0x10040).
				Build()
			writeToBottom = mem.WriteReqBuilder{}.
				WithAddress(0x10040).
				Build()

			t.inflightReqToBottom = []reqToBottom{
				{reqFromTop: readFromTop, reqToBottom: readToBottom},
				{reqFromTop: writeFromTop, reqToBottom: writeToBottom},
			}

		})

		It("should do nothing if there is no response to process", func() {
			bottomPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tMiddleware.respond()
			Expect(madeProgress).To(BeFalse())
		})

		It("should respond data ready", func() {
			dataReady := mem.DataReadyRspBuilder{}.
				WithRspTo(readToBottom.ID).
				Build()
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(dr *mem.DataReadyRsp) {
					Expect(dr.RespondTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})

		It("should respond write done", func() {
			done := mem.WriteDoneRspBuilder{}.
				WithRspTo(writeToBottom.ID).
				Build()
			bottomPort.EXPECT().PeekIncoming().Return(done)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(done *mem.WriteDoneRsp) {
					Expect(done.RespondTo).To(Equal(writeFromTop.ID))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := mem.DataReadyRspBuilder{}.
				WithRspTo(readToBottom.ID).
				Build()
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(dr *mem.DataReadyRsp) {
					Expect(dr.RespondTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(&sim.SendError{})

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeFalse())
			Expect(t.inflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("when handling control messages", func() {
		var (
			readFromTop   *mem.ReadReq
			writeFromTop  *mem.WriteReq
			readToBottom  *mem.ReadReq
			writeToBottom *mem.WriteReq
			flushReq      *mem.ControlMsg
			restartReq    *mem.ControlMsg
		)

		BeforeEach(func() {
			readFromTop = mem.ReadReqBuilder{}.
				WithAddress(0x10040).
				WithByteSize(4).
				Build()
			readToBottom = mem.ReadReqBuilder{}.
				WithAddress(0x20040).
				WithByteSize(4).
				Build()
			writeFromTop = mem.WriteReqBuilder{}.
				WithAddress(0x10040).
				Build()
			writeToBottom = mem.WriteReqBuilder{}.
				WithAddress(0x10040).
				Build()
			flushReq = mem.ControlMsgBuilder{}.
				WithDst(t.ctrlPort.AsRemote()).
				ToDiscardTransactions().
				Build()
			restartReq = mem.ControlMsgBuilder{}.
				WithDst(t.ctrlPort.AsRemote()).
				ToRestart().
				Build()

			t.inflightReqToBottom = []reqToBottom{
				{reqFromTop: readFromTop, reqToBottom: readToBottom},
				{reqFromTop: writeFromTop, reqToBottom: writeToBottom},
			}
		})

		It("should handle flush req", func() {
			ctrlPort.EXPECT().PeekIncoming().Return(flushReq)
			ctrlPort.EXPECT().RetrieveIncoming().Return(flushReq)
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := tMiddleware.handleCtrlRequest()

			Expect(madeProgress).To(BeTrue())
			Expect(t.isFlushing).To(BeTrue())
			Expect(t.inflightReqToBottom).To(BeNil())
		})

		It("should handle restart req", func() {
			ctrlPort.EXPECT().PeekIncoming().Return(restartReq)
			ctrlPort.EXPECT().RetrieveIncoming().Return(restartReq)
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)
			topPort.EXPECT().RetrieveIncoming().Return(nil)
			bottomPort.EXPECT().RetrieveIncoming().Return(nil)
			translationPort.EXPECT().RetrieveIncoming().Return(nil)

			madeProgress := tMiddleware.handleCtrlRequest()

			Expect(madeProgress).To(BeTrue())
			Expect(t.isFlushing).To(BeFalse())
		})

	})
})
