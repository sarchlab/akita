package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MemController", func() {
	var (
		mockCtrl *gomock.Controller

		topPort             *MockPort
		addrConverter       *MockAddressConverter
		subTransSplitter    *MockSubTransSplitter
		subTransactionQueue *MockSubTransactionQueue
		cmdQueue            *MockCommandQueue
		channel             *MockChannel
		storage             *mem.Storage

		memCtrl           *Comp
		memCtrlMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().Return(sim.RemotePort("TopPort")).AnyTimes()

		subTransactionQueue = NewMockSubTransactionQueue(mockCtrl)
		subTransSplitter = NewMockSubTransSplitter(mockCtrl)
		addrConverter = NewMockAddressConverter(mockCtrl)
		cmdQueue = NewMockCommandQueue(mockCtrl)
		channel = NewMockChannel(mockCtrl)
		storage = mem.NewStorage(4 * mem.GB)

		memCtrl = MakeBuilder().Build("MemCtrl")
		memCtrl.topPort = topPort
		memCtrl.subTransactionQueue = subTransactionQueue
		memCtrl.subTransSplitter = subTransSplitter
		memCtrl.addrConverter = addrConverter
		memCtrl.cmdQueue = cmdQueue
		memCtrl.channel = channel
		memCtrl.storage = storage
		memCtrlMiddleware = memCtrl.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("parse top", func() {
		It("should do nothing if no message", func() {
			topPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := memCtrlMiddleware.parseTop()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if substransaction queue is full", func() {
			read := mem.ReadReqBuilder{}.
				WithAddress(0x1000).
				Build()

			topPort.EXPECT().PeekIncoming().Return(read)
			addrConverter.EXPECT().ConvertExternalToInternal(uint64(0x1000))
			subTransSplitter.EXPECT().
				Split(gomock.Any()).
				Do(func(t *signal.Transaction) {
					Expect(t.Read).To(BeIdenticalTo(read))
					t.SubTransactions = make([]*signal.SubTransaction, 3)
				})
			subTransactionQueue.EXPECT().CanPush(3).Return(false)

			madeProgress := memCtrlMiddleware.parseTop()

			Expect(madeProgress).To(BeFalse())
		})

		It("should push sub-transactions to subtrans queue", func() {
			read := mem.ReadReqBuilder{}.
				WithAddress(0x1000).
				Build()

			topPort.EXPECT().PeekIncoming().Return(read)
			topPort.EXPECT().RetrieveIncoming().Return(read)
			addrConverter.EXPECT().ConvertExternalToInternal(uint64(0x1000))
			subTransSplitter.EXPECT().
				Split(gomock.Any()).
				Do(func(t *signal.Transaction) {
					Expect(t.Read).To(BeIdenticalTo(read))
					for i := 0; i < 3; i++ {
						st := &signal.SubTransaction{}
						t.SubTransactions = append(t.SubTransactions, st)
					}
				})
			subTransactionQueue.EXPECT().CanPush(3).Return(true)
			subTransactionQueue.EXPECT().Push(gomock.Any())

			madeProgress := memCtrlMiddleware.parseTop()

			Expect(madeProgress).To(BeTrue())
			Expect(memCtrl.inflightTransactions).To(HaveLen(1))
		})

	})

	Context("issue", func() {
		It("should not issue if nothing is ready", func() {
			cmdQueue.EXPECT().
				GetCommandToIssue().
				Return(nil)

			madeProgress := memCtrlMiddleware.issue()

			Expect(madeProgress).To(BeFalse())
		})

		It("should issue", func() {
			cmd := &signal.Command{}
			cmdQueue.EXPECT().
				GetCommandToIssue().
				Return(cmd)
			channel.EXPECT().StartCommand(cmd)
			channel.EXPECT().UpdateTiming(cmd)

			madeProgress := memCtrlMiddleware.issue()

			Expect(madeProgress).To(BeTrue())
		})
	})

	Context("respond", func() {
		It("should do nothing if there is no transaction", func() {
			madeProgress := memCtrlMiddleware.respond()

			Expect(madeProgress).To(BeFalse())
		})

		It("should do nothing if there is no completed transaction",
			func() {
				trans := &signal.Transaction{}
				subTransaction := &signal.SubTransaction{
					Transaction: trans,
					Completed:   false,
				}
				trans.SubTransactions = append(trans.SubTransactions,
					subTransaction)
				memCtrl.inflightTransactions = append(
					memCtrl.inflightTransactions, trans)

				madeProgress := memCtrlMiddleware.respond()

				Expect(madeProgress).To(BeFalse())
			})

		It("should send write done response", func() {
			write := mem.WriteReqBuilder{}.
				WithAddress(0x40).
				WithData([]byte{1, 2, 3, 4}).
				Build()
			trans := &signal.Transaction{
				InternalAddress: 0x40,
				Write:           write,
			}
			subTransaction := &signal.SubTransaction{
				Transaction: trans,
				Completed:   true,
			}
			trans.SubTransactions = append(trans.SubTransactions,
				subTransaction)
			memCtrl.inflightTransactions = append(memCtrl.inflightTransactions,
				trans)

			topPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := memCtrlMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			data, _ := storage.Read(0x40, 4)
			Expect(data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(memCtrl.inflightTransactions).NotTo(ContainElement(trans))
		})

		It("should send data ready response", func() {
			storage.Write(0x40, []byte{1, 2, 3, 4})
			read := mem.ReadReqBuilder{}.
				WithAddress(0x40).
				WithByteSize(4).
				Build()
			trans := &signal.Transaction{
				InternalAddress: 0x40,
				Read:            read,
			}
			subTransaction := &signal.SubTransaction{
				Transaction: trans,
				Completed:   true,
			}
			trans.SubTransactions = append(trans.SubTransactions,
				subTransaction)
			memCtrl.inflightTransactions = append(memCtrl.inflightTransactions,
				trans)

			topPort.EXPECT().Send(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			}).Return(nil)

			madeProgress := memCtrlMiddleware.respond()

			Expect(madeProgress).To(BeTrue())
			Expect(memCtrl.inflightTransactions).NotTo(ContainElement(trans))
		})
	})
})
