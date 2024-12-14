package addresstranslator

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

var _ = Describe("Address Translator", func() {
	var (
		mockCtrl            *gomock.Controller
		topPort             *MockPort
		bottomPort          *MockPort
		translationPort     *MockPort
		ctrlPort            *MockPort
		addressToPortMapper *MockAddressToPortMapper

		t           *Comp
		tMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("TopPort")).
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("BottomPort")).
			AnyTimes()
		ctrlPort = NewMockPort(mockCtrl)
		ctrlPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("CtrlPort")).
			AnyTimes()
		translationPort = NewMockPort(mockCtrl)
		translationPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("TranslationPort")).
			AnyTimes()
		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		builder := MakeBuilder().
			WithLog2PageSize(12).
			WithFreq(1).
			WithAddressToPortMapper(addressToPortMapper)
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
			req mem.ReadReq
		)

		BeforeEach(func() {
			req = mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x100,
				AccessByteSize: 4,
				PID:            1,
			}
		})

		It("should do nothing if there is no request", func() {
			topPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tMiddleware.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			var transReqReturn vm.TranslationReq
			transReq := vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}

			translation := &transaction{
				translationReq: transReq,
			}
			t.transactions = append(t.transactions, translation)
			req.Address = 0x1040

			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().RetrieveIncoming()
			translationPort.EXPECT().Send(gomock.Any()).
				DoAndReturn(func(req vm.TranslationReq) *modeling.SendError {
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
			topPort.EXPECT().PeekIncoming().Return(req)
			translationPort.EXPECT().
				Send(gomock.Any()).
				Return(&modeling.SendError{})

			needTick := tMiddleware.translate()

			Expect(needTick).To(BeFalse())
			Expect(t.transactions).To(HaveLen(0))
		})
	})

	Context("parse translation", func() {
		var (
			transReq1, transReq2 vm.TranslationReq
			trans1, trans2       *transaction
		)

		BeforeEach(func() {
			transReq1 = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}
			trans1 = &transaction{
				translationReq: transReq1,
			}
			transReq2 = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}
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
			req := mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x10040,
				AccessByteSize: 4,
				PID:            1,
			}
			translationRsp := vm.TranslationRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				RespondTo: transReq1.ID,
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}

			trans1.incomingReqs = []mem.AccessReq{req}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			addressToPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Return(modeling.NewSendError())

			madeProgress := tMiddleware.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x10040,
				AccessByteSize: 4,
				PID:            1,
			}
			translationRsp := vm.TranslationRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				RespondTo: transReq1.ID,
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}

			trans1.incomingReqs = []mem.AccessReq{req}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			addressToPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(read mem.ReadReq) {
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
			write := mem.WriteReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:   0x10040,
				PID:       1,
				Data:      data,
				DirtyMask: dirty,
			}
			translationRsp := vm.TranslationRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				RespondTo: transReq1.ID,
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			trans1.incomingReqs = []mem.AccessReq{write}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			addressToPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(req mem.WriteReq) {
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
			readFromTop   mem.ReadReq
			writeFromTop  mem.WriteReq
			readToBottom  mem.ReadReq
			writeToBottom mem.WriteReq
		)

		BeforeEach(func() {
			readFromTop = mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x10040,
				AccessByteSize: 4,
				PID:            1,
			}
			readToBottom = mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x20040,
				AccessByteSize: 4,
				PID:            1,
			}
			writeFromTop = mem.WriteReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address: 0x10040,
				PID:     1,
			}
			writeToBottom = mem.WriteReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address: 0x10040,
				PID:     1,
			}

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
			dataReady := mem.DataReadyRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				RespondTo: readToBottom.ID,
				Data:      []byte{1, 2, 3, 4},
			}
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(dr mem.DataReadyRsp) {
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
			done := mem.WriteDoneRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				RespondTo: writeToBottom.ID,
			}
			bottomPort.EXPECT().PeekIncoming().Return(done)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(done mem.WriteDoneRsp) {
					Expect(done.RespondTo).To(Equal(writeFromTop.ID))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := mem.DataReadyRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				RespondTo: readToBottom.ID,
				Data:      []byte{1, 2, 3, 4},
			}
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(dr mem.DataReadyRsp) {
					Expect(dr.RespondTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(&modeling.SendError{})

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeFalse())
			Expect(t.inflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("when handling control messages", func() {
		var (
			readFromTop   mem.ReadReq
			writeFromTop  mem.WriteReq
			readToBottom  mem.ReadReq
			writeToBottom mem.WriteReq
			flushReq      mem.ControlMsg
			restartReq    mem.ControlMsg
		)

		BeforeEach(func() {
			readFromTop = mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x10040,
				AccessByteSize: 4,
				PID:            1,
			}
			readToBottom = mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address:        0x20040,
				AccessByteSize: 4,
				PID:            1,
			}
			writeFromTop = mem.WriteReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address: 0x10040,
				PID:     1,
			}
			writeToBottom = mem.WriteReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Address: 0x10040,
				PID:     1,
			}
			flushReq = mem.ControlMsg{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				DiscardTransactions: true,
			}
			restartReq = mem.ControlMsg{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: bottomPort.AsRemote(),
					Dst: topPort.AsRemote(),
				},
				Restart: true,
			}

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
