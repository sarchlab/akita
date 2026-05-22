package addresstranslator

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Address Translator", func() {
	var (
		mockCtrl        *gomock.Controller
		topPort         *MockPort
		bottomPort      *MockPort
		translationPort *MockPort
		ctrlPort        *MockPort

		t              *modeling.Component[Spec, State]
		tParseTransMW  *parseTranslateMW
		tRespondPipeMW *respondPipelineMW
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("TopPort")).
			AnyTimes()
		topPort.EXPECT().
			Name().
			Return("TopPort").
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("BottomPort")).
			AnyTimes()
		bottomPort.EXPECT().
			Name().
			Return("BottomPort").
			AnyTimes()
		ctrlPort = NewMockPort(mockCtrl)
		ctrlPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("CtrlPort")).
			AnyTimes()
		translationPort = NewMockPort(mockCtrl)
		translationPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("TranslationPort")).
			AnyTimes()
		translationPort.EXPECT().
			Name().
			Return("TranslationPort").
			AnyTimes()

		topPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		bottomPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		translationPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		ctrlPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		builder := MakeBuilder().
			WithLog2PageSize(12).
			WithFreq(1).
			WithMemoryProviderType("single").
			WithMemoryProviders(messaging.RemotePort("MemPort")).
			WithTranslationProviderMapperType("single").
			WithTranslationProviders(messaging.RemotePort("TranslationPort")).
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithTranslationPort(translationPort).
			WithCtrlPort(ctrlPort)

		t = builder.Build("AddressTranslator")

		tParseTransMW = t.Middlewares()[0].(*parseTranslateMW)
		tRespondPipeMW = t.Middlewares()[1].(*respondPipelineMW)
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
			req.ID = timing.GetIDGenerator().Generate()
			req.Address = 0x100
			req.AccessByteSize = 4
			req.PID = 1
			req.TrafficBytes = 12
			req.TrafficClass = "mem.ReadReq"
		})

		It("should do nothing if there is no request", func() {
			topPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tParseTransMW.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			var transReqReturn *vm.TranslationReq
			transReq := &vm.TranslationReq{}
			transReq.ID = timing.GetIDGenerator().Generate()
			transReq.PID = 1
			transReq.VAddr = 0x100
			transReq.DeviceID = 1
			transReq.TrafficClass = "vm.TranslationReq"

			// Set initial state with an existing transaction
			nextState := &t.State
			nextState.Transactions = append(nextState.Transactions, transactionState{
				TranslationReqID: transReq.ID,
			})

			req.Address = 0x1040

			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().RetrieveIncoming()
			translationPort.EXPECT().Send(gomock.Any()).
				DoAndReturn(func(msg messaging.Msg) *messaging.SendError {
					transReqReturn = msg.(*vm.TranslationReq)
					return nil
				})

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(2))
			Expect(updatedState.Transactions[1].TranslationReqID).
				To(Equal(transReqReturn.ID))
		})

		It("should stall if cannot send for translation", func() {
			topPort.EXPECT().PeekIncoming().Return(req)
			translationPort.EXPECT().
				Send(gomock.Any()).
				Return(&messaging.SendError{})

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(0))
		})
	})

	Context("parse translation", func() {
		var (
			transReq1, transReq2 *vm.TranslationReq
		)

		BeforeEach(func() {
			transReq1 = &vm.TranslationReq{}
			transReq1.ID = timing.GetIDGenerator().Generate()
			transReq1.PID = 1
			transReq1.VAddr = 0x100
			transReq1.DeviceID = 1
			transReq1.TrafficClass = "vm.TranslationReq"
			transReq2 = &vm.TranslationReq{}
			transReq2.ID = timing.GetIDGenerator().Generate()
			transReq2.PID = 1
			transReq2.VAddr = 0x100
			transReq2.DeviceID = 1
			transReq2.TrafficClass = "vm.TranslationReq"

			t.State = State{
				Transactions: []transactionState{
					{TranslationReqID: transReq1.ID},
					{TranslationReqID: transReq2.ID},
				},
			}
		})

		It("should do nothing if there is no translation return", func() {
			translationPort.EXPECT().PeekIncoming().Return(nil)
			needTick := tRespondPipeMW.parseTranslation()
			Expect(needTick).To(BeFalse())
		})

		It("should stall if send failed", func() {
			req := &mem.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
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
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req),
						},
						TranslationDone: true,
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			bottomPort.EXPECT().Send(gomock.Any()).Return(messaging.NewSendError())

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := &mem.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
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
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req),
						},
						TranslationDone: true,
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					read := msg.(*mem.ReadReq)
					Expect(read.PID).To(Equal(vm.PID(0)))
					Expect(read.Address).To(Equal(uint64(0x20040)))
					Expect(read.AccessByteSize).To(Equal(uint64(4)))
					Expect(read.Src).To(Equal(bottomPort.AsRemote()))
				}).
				Return(nil)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.Transactions).NotTo(
				ContainElement(
					WithTransform(
						func(ts transactionState) uint64 { return ts.TranslationReqID },
						Equal(transReq1.ID),
					),
				),
			)
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should forward write request", func() {
			data := []byte{1, 2, 3, 4}
			dirty := []bool{false, true, false, true}
			write := &mem.WriteReq{}
			write.ID = timing.GetIDGenerator().Generate()
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
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vm.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(write),
						},
						TranslationDone: true,
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.EXPECT().PeekIncoming().Return(translationRsp)
			translationPort.EXPECT().RetrieveIncoming()
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					writeMsg := msg.(*mem.WriteReq)
					Expect(writeMsg.PID).To(Equal(vm.PID(0)))
					Expect(writeMsg.Address).To(Equal(uint64(0x20040)))
					Expect(writeMsg.Src).To(Equal(bottomPort.AsRemote()))
					Expect(writeMsg.Data).To(Equal(data))
					Expect(writeMsg.DirtyMask).To(Equal(dirty))
				}).
				Return(nil)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
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
			readFromTop.ID = timing.GetIDGenerator().Generate()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "mem.ReadReq"
			readToBottom = &mem.ReadReq{}
			readToBottom.ID = timing.GetIDGenerator().Generate()
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = timing.GetIDGenerator().Generate()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "mem.WriteReq"
			writeToBottom = &mem.WriteReq{}
			writeToBottom.ID = timing.GetIDGenerator().Generate()
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"

			t.State = State{
				InflightReqToBottom: []reqToBottomState{
					{
						ReqFromTopID:    readFromTop.ID,
						ReqFromTopSrc:   readFromTop.Src,
						ReqFromTopDst:   readFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", readFromTop),
						ReqToBottomID:   readToBottom.ID,
						ReqToBottomSrc:  readToBottom.Src,
						ReqToBottomDst:  readToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", readToBottom),
					},
					{
						ReqFromTopID:    writeFromTop.ID,
						ReqFromTopSrc:   writeFromTop.Src,
						ReqFromTopDst:   writeFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", writeFromTop),
						ReqToBottomID:   writeToBottom.ID,
						ReqToBottomSrc:  writeToBottom.Src,
						ReqToBottomDst:  writeToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", writeToBottom),
					},
				},
			}
		})

		It("should do nothing if there is no response to process", func() {
			bottomPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tRespondPipeMW.respond()
			Expect(madeProgress).To(BeFalse())
		})

		It("should respond data ready", func() {
			dataReady := &mem.DataReadyRsp{}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "mem.DataReadyRsp"
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should respond write done", func() {
			done := &mem.WriteDoneRsp{}
			done.ID = timing.GetIDGenerator().Generate()
			done.RspTo = writeToBottom.ID
			done.TrafficBytes = 4
			done.TrafficClass = "mem.WriteDoneRsp"
			bottomPort.EXPECT().PeekIncoming().Return(done)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					doneMsg := msg.(*mem.WriteDoneRsp)
					Expect(doneMsg.RspTo).To(Equal(writeFromTop.ID))
				}).
				Return(nil)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := &mem.DataReadyRsp{}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "mem.DataReadyRsp"
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(readFromTop.ID))
					Expect(dr.Data).To(Equal(dataReady.Data))
				}).
				Return(&messaging.SendError{})

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("state serialization", func() {
		It("should pass ValidateState", func() {
			err := modeling.ValidateState(State{})
			Expect(err).To(Succeed())
		})
	})

	Context("when handling control messages", func() {
		var (
			readFromTop   *mem.ReadReq
			writeFromTop  *mem.WriteReq
			readToBottom  *mem.ReadReq
			writeToBottom *mem.WriteReq
			flushReq      *mem.ControlReq
			restartReq    *mem.ControlReq
		)

		BeforeEach(func() {
			readFromTop = &mem.ReadReq{}
			readFromTop.ID = timing.GetIDGenerator().Generate()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "mem.ReadReq"
			readToBottom = &mem.ReadReq{}
			readToBottom.ID = timing.GetIDGenerator().Generate()
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = timing.GetIDGenerator().Generate()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "mem.WriteReq"
			writeToBottom = &mem.WriteReq{}
			writeToBottom.ID = timing.GetIDGenerator().Generate()
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"
			flushReq = &mem.ControlReq{
				Command: mem.CmdFlush,
			}
			flushReq.ID = timing.GetIDGenerator().Generate()
			flushReq.Dst = ctrlPort.AsRemote()
			flushReq.TrafficBytes = 4
			flushReq.TrafficClass = "mem.ControlReq"
			restartReq = &mem.ControlReq{
				Command: mem.CmdReset,
			}
			restartReq.ID = timing.GetIDGenerator().Generate()
			restartReq.Dst = ctrlPort.AsRemote()
			restartReq.TrafficBytes = 4
			restartReq.TrafficClass = "mem.ControlReq"

			nextState := &t.State
			nextState.InflightReqToBottom = []reqToBottomState{
				{
					ReqFromTopID:    readFromTop.ID,
					ReqFromTopSrc:   readFromTop.Src,
					ReqFromTopDst:   readFromTop.Dst,
					ReqFromTopType:  fmt.Sprintf("%T", readFromTop),
					ReqToBottomID:   readToBottom.ID,
					ReqToBottomSrc:  readToBottom.Src,
					ReqToBottomDst:  readToBottom.Dst,
					ReqToBottomType: fmt.Sprintf("%T", readToBottom),
				},
				{
					ReqFromTopID:    writeFromTop.ID,
					ReqFromTopSrc:   writeFromTop.Src,
					ReqFromTopDst:   writeFromTop.Dst,
					ReqFromTopType:  fmt.Sprintf("%T", writeFromTop),
					ReqToBottomID:   writeToBottom.ID,
					ReqToBottomSrc:  writeToBottom.Src,
					ReqToBottomDst:  writeToBottom.Dst,
					ReqToBottomType: fmt.Sprintf("%T", writeToBottom),
				},
			}
		})

		It("should handle flush req", func() {
			ctrlPort.EXPECT().PeekIncoming().Return(flushReq)
			ctrlPort.EXPECT().RetrieveIncoming().Return(flushReq)
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := tParseTransMW.handleCtrlRequest()

			Expect(madeProgress).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.IsFlushing).To(BeTrue())
			Expect(updatedState.InflightReqToBottom).To(BeNil())
		})

		It("should handle restart req", func() {
			ctrlPort.EXPECT().PeekIncoming().Return(restartReq)
			ctrlPort.EXPECT().RetrieveIncoming().Return(restartReq)
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)
			topPort.EXPECT().RetrieveIncoming().Return(nil)
			bottomPort.EXPECT().RetrieveIncoming().Return(nil)
			translationPort.EXPECT().RetrieveIncoming().Return(nil)

			madeProgress := tParseTransMW.handleCtrlRequest()

			Expect(madeProgress).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.IsFlushing).To(BeFalse())
		})

	})
})
