package writearound

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
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
			bankBufs:         []queueing.Buffer{bankBuf},
		}
		c.TickingComponent = modeling.NewTickingComponent(
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
			write1 := mem.WriteReq{
				MsgMeta: modeling.MsgMeta{},
				Address: 0x100,
				PID:     1,
			}
			preCTrans1 := &transaction{
				write: write1,
			}
			write2 := mem.WriteReq{
				MsgMeta: modeling.MsgMeta{},
				Address: 0x104,
				PID:     1,
			}
			preCTrans2 := &transaction{
				write: write2,
			}
			writeToBottom := mem.WriteReq{
				MsgMeta: modeling.MsgMeta{},
				Address: 0x100,
				PID:     1,
			}
			postCTrans := &transaction{
				writeToBottom:           writeToBottom,
				preCoalesceTransactions: []*transaction{preCTrans1, preCTrans2},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)
			done := mem.WriteDoneRsp{
				MsgMeta:   modeling.MsgMeta{},
				RespondTo: writeToBottom.ID,
			}

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
			read1, read2             mem.ReadReq
			write1, write2           mem.WriteReq
			preCTrans1, preCTrans2   *transaction
			preCTrans3, preCTrans4   *transaction
			postCRead                mem.ReadReq
			postCWrite               mem.WriteReq
			readToBottom             mem.ReadReq
			block                    *cache.Block
			postCTrans1, postCTrans2 *transaction
			mshrEntry                *cache.MSHREntry
			dataReady                mem.DataReadyRsp
		)

		BeforeEach(func() {
			read1 = mem.ReadReq{
				MsgMeta:        modeling.MsgMeta{},
				Address:        0x100,
				PID:            1,
				AccessByteSize: 4,
			}
			read2 = mem.ReadReq{
				MsgMeta:        modeling.MsgMeta{},
				Address:        0x104,
				PID:            1,
				AccessByteSize: 4,
			}
			write1 = mem.WriteReq{
				MsgMeta: modeling.MsgMeta{},
				Address: 0x108,
				PID:     1,
				Data:    []byte{9, 9, 9, 9},
				DirtyMask: []bool{
					false, false, false, false,
					true, true, true, true,
				},
			}
			write2 = mem.WriteReq{
				MsgMeta:   modeling.MsgMeta{},
				Address:   0x10C,
				PID:       1,
				Data:      []byte{9, 9, 9, 9},
				DirtyMask: []bool{false, false, false, false, true, true, true, true},
			}

			preCTrans1 = &transaction{read: read1}
			preCTrans2 = &transaction{read: read2}
			preCTrans3 = &transaction{write: write1}
			preCTrans4 = &transaction{write: write2}

			postCRead = mem.ReadReq{
				MsgMeta:        modeling.MsgMeta{},
				Address:        0x100,
				PID:            1,
				AccessByteSize: 64,
			}
			readToBottom = mem.ReadReq{
				MsgMeta:        modeling.MsgMeta{},
				Address:        0x100,
				PID:            1,
				AccessByteSize: 64,
			}
			dataReady = mem.DataReadyRsp{
				MsgMeta:   modeling.MsgMeta{},
				RespondTo: readToBottom.ID,
				Data: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
			}

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

			postCWrite = mem.WriteReq{
				MsgMeta: modeling.MsgMeta{},
				Address: 0x100,
				PID:     1,
				Data: []byte{
					0, 0, 0, 0, 0, 0, 0, 0,
					9, 9, 9, 9, 9, 9, 9, 9,
				},
				DirtyMask: []bool{
					false, false, false, false, false, false, false, false,
					true, true, true, true, true, true, true, true,
				},
			}
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
