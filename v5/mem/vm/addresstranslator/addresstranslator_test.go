package addresstranslator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
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

		topPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		bottomPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		translationPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		ctrlPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		builder := MakeBuilder().
			WithLog2PageSize(12).
			WithFreq(1).
			WithMemoryProviderMapper(memoryPortMapper).
			WithTranslationProviderMapper(translationPortMapper).
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithTranslationPort(translationPort).
			WithCtrlPort(ctrlPort)

		t = builder.Build("AddressTranslator")

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
			req = &mem.ReadReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Address = 0x100
			req.AccessByteSize = 4
			req.PID = 1
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
		})

		It("should do nothing if there is no request", func() {
			topPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tMiddleware.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			var transReqReturn *vm.TranslationReq
			transReq := &vm.TranslationReq{}
			transReq.ID = sim.GetIDGenerator().Generate()
			transReq.PID = 1
			transReq.VAddr = 0x100
			transReq.DeviceID = 1
			transReq.TrafficClass = "vm.TranslationReq"

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
				DoAndReturn(func(msg sim.Msg) *sim.SendError {
					transReqReturn = msg.(*vm.TranslationReq)
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
			transReq1 = &vm.TranslationReq{}
			transReq1.ID = sim.GetIDGenerator().Generate()
			transReq1.PID = 1
			transReq1.VAddr = 0x100
			transReq1.DeviceID = 1
			transReq1.TrafficClass = "vm.TranslationReq"
			trans1 = &transaction{
				translationReq: transReq1,
			}
			transReq2 = &vm.TranslationReq{}
			transReq2.ID = sim.GetIDGenerator().Generate()
			transReq2.PID = 1
			transReq2.VAddr = 0x100
			transReq2.DeviceID = 1
			transReq2.TrafficClass = "vm.TranslationReq"
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
			req := &mem.ReadReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Address = 0x10040
			req.AccessByteSize = 4
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
			translationRsp := &vm.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = sim.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			trans1.incomingReqs = []sim.Msg{req}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			memoryPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).Return(sim.NewSendError())

			madeProgress := tMiddleware.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := &mem.ReadReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Address = 0x10040
			req.AccessByteSize = 4
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
			translationRsp := &vm.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = sim.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			trans1.incomingReqs = []sim.Msg{req}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			memoryPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					read := msg.(*mem.ReadReq)
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
			write := &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x10040
			write.Data = data
			write.DirtyMask = dirty
			write.TrafficBytes = len(data) + 12
			write.TrafficClass = "mem.WriteReq"
			translationRsp := &vm.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = sim.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"
			trans1.incomingReqs = []sim.Msg{write}
			trans1.translationRsp = translationRsp
			trans1.translationDone = true

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			memoryPortMapper.EXPECT().Find(uint64(0x20040))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					writeMsg := msg.(*mem.WriteReq)
					Expect(writeMsg).NotTo(BeIdenticalTo(write))
					Expect(writeMsg.PID).To(Equal(vm.PID(0)))
					Expect(writeMsg.Address).To(Equal(uint64(0x20040)))
					Expect(writeMsg.Src).To(Equal(bottomPort.AsRemote()))
					Expect(writeMsg.Data).To(Equal(data))
					Expect(writeMsg.DirtyMask).To(Equal(dirty))
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
			readFromTop = &mem.ReadReq{}
			readFromTop.ID = sim.GetIDGenerator().Generate()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "mem.ReadReq"
			readToBottom = &mem.ReadReq{}
			readToBottom.ID = sim.GetIDGenerator().Generate()
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = sim.GetIDGenerator().Generate()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "mem.WriteReq"
			writeToBottom = &mem.WriteReq{}
			writeToBottom.ID = sim.GetIDGenerator().Generate()
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"

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
			dataReady := &mem.DataReadyRsp{}
			dataReady.ID = sim.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "mem.DataReadyRsp"
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})

		It("should respond write done", func() {
			done := &mem.WriteDoneRsp{}
			done.ID = sim.GetIDGenerator().Generate()
			done.RspTo = writeToBottom.ID
			done.TrafficBytes = 4
			done.TrafficClass = "mem.WriteDoneRsp"
			bottomPort.EXPECT().PeekIncoming().Return(done)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					doneMsg := msg.(*mem.WriteDoneRsp)
					Expect(doneMsg.RspTo).To(Equal(writeFromTop.ID))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			Expect(t.inflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := &mem.DataReadyRsp{}
			dataReady.ID = sim.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "mem.DataReadyRsp"
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(&sim.SendError{})

			madeProgress := tMiddleware.respond()

			Expect(madeProgress).To(BeFalse())
			Expect(t.inflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("state serialization", func() {
		It("should pass ValidateState", func() {
			err := modeling.ValidateState(State{})
			Expect(err).To(Succeed())
		})

		It("should round-trip GetState/SetState", func() {
			reqFromTop := &mem.ReadReq{}
			reqFromTop.ID = sim.GetIDGenerator().Generate()
			reqFromTop.Address = 0x10040
			reqFromTop.AccessByteSize = 4
			reqFromTop.TrafficBytes = 12
			reqFromTop.TrafficClass = "mem.ReadReq"
			reqToBot := &mem.ReadReq{}
			reqToBot.ID = sim.GetIDGenerator().Generate()
			reqToBot.Address = 0x20040
			reqToBot.AccessByteSize = 4
			reqToBot.TrafficBytes = 12
			reqToBot.TrafficClass = "mem.ReadReq"

			transReq := &vm.TranslationReq{}
			transReq.ID = sim.GetIDGenerator().Generate()
			transReq.PID = 1
			transReq.VAddr = 0x100
			transReq.DeviceID = 1
			transReq.TrafficClass = "vm.TranslationReq"

			t.isFlushing = true
			t.transactions = []*transaction{
				{
					incomingReqs:    []sim.Msg{reqFromTop},
					translationReq:  transReq,
					translationDone: true,
				},
			}
			t.inflightReqToBottom = []reqToBottom{
				{reqFromTop: reqFromTop, reqToBottom: reqToBot},
			}

			state := t.GetState()

			Expect(state.IsFlushing).To(BeTrue())
			Expect(state.Transactions).To(HaveLen(1))
			Expect(state.Transactions[0].TranslationReqID).
				To(Equal(transReq.ID))
			Expect(state.Transactions[0].TranslationDone).To(BeTrue())
			Expect(state.Transactions[0].IncomingReqs).To(HaveLen(1))
			Expect(state.Transactions[0].IncomingReqs[0].ID).
				To(Equal(reqFromTop.ID))
			Expect(state.InflightReqToBottom).To(HaveLen(1))
			Expect(state.InflightReqToBottom[0].ReqFromTopID).
				To(Equal(reqFromTop.ID))
			Expect(state.InflightReqToBottom[0].ReqToBottomID).
				To(Equal(reqToBot.ID))

			// Clear and restore
			t.isFlushing = false
			t.transactions = nil
			t.inflightReqToBottom = nil

			t.SetState(state)

			Expect(t.isFlushing).To(BeTrue())
			Expect(t.transactions).To(HaveLen(1))
			Expect(t.transactions[0].translationReq.ID).
				To(Equal(transReq.ID))
			Expect(t.transactions[0].translationDone).To(BeTrue())
			Expect(t.transactions[0].incomingReqs).To(HaveLen(1))
			Expect(t.transactions[0].incomingReqs[0].Meta().ID).
				To(Equal(reqFromTop.ID))
			Expect(t.inflightReqToBottom).To(HaveLen(1))
			Expect(t.inflightReqToBottom[0].reqFromTop.Meta().ID).
				To(Equal(reqFromTop.ID))
			Expect(t.inflightReqToBottom[0].reqToBottom.Meta().ID).
				To(Equal(reqToBot.ID))
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
			readFromTop = &mem.ReadReq{}
			readFromTop.ID = sim.GetIDGenerator().Generate()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "mem.ReadReq"
			readToBottom = &mem.ReadReq{}
			readToBottom.ID = sim.GetIDGenerator().Generate()
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = sim.GetIDGenerator().Generate()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "mem.WriteReq"
			writeToBottom = &mem.WriteReq{}
			writeToBottom.ID = sim.GetIDGenerator().Generate()
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"
			flushReq = &mem.ControlMsg{
				DiscardTransations: true,
			}
			flushReq.ID = sim.GetIDGenerator().Generate()
			flushReq.Dst = t.ctrlPort.AsRemote()
			flushReq.TrafficBytes = 4
			flushReq.TrafficClass = "mem.ControlMsg"
			restartReq = &mem.ControlMsg{
				Restart: true,
			}
			restartReq.ID = sim.GetIDGenerator().Generate()
			restartReq.Dst = t.ctrlPort.AsRemote()
			restartReq.TrafficBytes = 4
			restartReq.TrafficClass = "mem.ControlMsg"

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
