package writethrough

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Bottom Parser", func() {
	var (
		mockCtrl   *gomock.Controller
		bottomPort *MockPort
		bankBuf    *MockBuffer
		mshr       *MockMSHR
		p          *bottomParser
		c          *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		c = &Comp{
			log2BlockSize:    6,
			bottomPort:       bottomPort,
			mshr:             mshr,
			wayAssociativity: 4,
			bankBufs:         []sim.Buffer{bankBuf},
		}
		c.TickingComponent = sim.NewTickingComponent(
			"Cache", nil, 1, c)
		p = &bottomParser{cache: c}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no respond", func() {
		bottomPort.EXPECT().PeekIncoming().Return(nil)
		madeProgress := p.Tick()
		Expect(madeProgress).To(BeFalse())
	})

	Context("write done", func() {
		It("should handle write done", func() {
			write1 := mem.WriteReqBuilder{}.
				WithAddress(0x100).
				WithPID(1).
				Build()
			preCTrans1 := &transaction{
				write: write1,
			}
			write2 := mem.WriteReqBuilder{}.
				WithAddress(0x104).
				WithPID(1).
				Build()
			preCTrans2 := &transaction{
				write: write2,
			}
			writeToBottom := mem.WriteReqBuilder{}.
				WithAddress(0x100).
				WithPID(1).
				Build()
			postCTrans := &transaction{
				writeToBottom:           writeToBottom,
				preCoalesceTransactions: []*transaction{preCTrans1, preCTrans2},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)
			done := mem.WriteDoneRspBuilder{}.
				WithRspTo(writeToBottom.ID).
				Build()

			bottomPort.EXPECT().PeekIncoming().Return(done)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.done).To(BeTrue())
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans))
		})
	})

	Context("data ready", func() {
		var (
			read1, read2             *mem.ReadReq
			write1, write2           *mem.WriteReq
			preCTrans1, preCTrans2   *transaction
			preCTrans3, preCTrans4   *transaction
			postCRead                *mem.ReadReq
			postCWrite               *mem.WriteReq
			readToBottom             *mem.ReadReq
			block                    *cache.Block
			postCTrans1, postCTrans2 *transaction
			mshrEntry                *cache.MSHREntry
			dataReady                *mem.DataReadyRsp
		)

		BeforeEach(func() {
			read1 = mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithPID(1).
				WithByteSize(4).
				Build()
			read2 = mem.ReadReqBuilder{}.
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()
			write1 = mem.WriteReqBuilder{}.
				WithAddress(0x108).
				WithPID(1).
				WithData([]byte{9, 9, 9, 9}).
				Build()
			write2 = mem.WriteReqBuilder{}.
				WithAddress(0x10C).
				WithPID(1).
				WithData([]byte{9, 9, 9, 9}).
				Build()

			preCTrans1 = &transaction{read: read1}
			preCTrans2 = &transaction{read: read2}
			preCTrans3 = &transaction{write: write1}
			preCTrans4 = &transaction{write: write2}

			postCRead = mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithPID(1).
				WithByteSize(64).
				Build()
			readToBottom = mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithPID(1).
				WithByteSize(64).
				Build()

			dataReady = mem.DataReadyRspBuilder{}.
				WithRspTo(readToBottom.ID).
				WithData([]byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				}).
				Build()
			block = &cache.Block{
				PID: 1,
				Tag: 0x100,
			}
			postCTrans1 = &transaction{
				block:        block,
				read:         postCRead,
				readToBottom: readToBottom,
				preCoalesceTransactions: []*transaction{
					preCTrans1,
					preCTrans2,
				},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans1)

			postCWrite = mem.WriteReqBuilder{}.
				WithAddress(0x100).
				WithPID(1).
				WithData([]byte{
					0, 0, 0, 0, 0, 0, 0, 0,
					9, 9, 9, 9, 9, 9, 9, 9,
				}).
				WithDirtyMask([]bool{
					false, false, false, false, false, false, false, false,
					true, true, true, true, true, true, true, true,
				}).
				Build()
			postCTrans2 = &transaction{
				write: postCWrite,
				preCoalesceTransactions: []*transaction{
					preCTrans3, preCTrans4,
				},
			}

			mshrEntry = &cache.MSHREntry{
				Block: block,
			}
			mshrEntry.Requests = append(mshrEntry.Requests, postCTrans1)
		})

		It("should stall is bank is busy", func() {
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send transaction to bank", func() {
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			mshr.EXPECT().Remove(vm.PID(1), uint64(0x100))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
				})

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(c.postCoalesceTransactions).
				NotTo(ContainElement(postCTrans1))
		})

		It("should combine write", func() {
			mshrEntry.Requests = append(mshrEntry.Requests, postCTrans2)
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans2)

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			mshr.EXPECT().Remove(vm.PID(1), uint64(0x100))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
					Expect(trans.data).To(Equal([]byte{
						1, 2, 3, 4, 5, 6, 7, 8,
						9, 9, 9, 9, 9, 9, 9, 9,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
					}))
					Expect(trans.writeFetchedDirtyMask).To(Equal([]bool{
						false, false, false, false, false, false, false, false,
						true, true, true, true, true, true, true, true,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
					}))
				})

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans3.done).To(BeTrue())
			Expect(preCTrans4.done).To(BeTrue())
			Expect(c.postCoalesceTransactions).
				NotTo(ContainElement(postCTrans1))
			Expect(c.postCoalesceTransactions).
				NotTo(ContainElement(postCTrans2))
		})
	})

})
