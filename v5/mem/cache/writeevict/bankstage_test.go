package writeevict

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Bankstage", func() {
	var (
		mockCtrl        *gomock.Controller
		inBuf           *MockBuffer
		storage         *mem.Storage
		pipeline        *MockPipeline
		postPipelineBuf *MockBuffer
		s               *bankStage
		c               *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		storage = mem.NewStorage(4 * mem.KB)
		pipeline = NewMockPipeline(mockCtrl)
		postPipelineBuf = NewMockBuffer(mockCtrl)
		c = &middleware{
			bankBufs: []queueing.Buffer{inBuf},
			storage:  storage,
		}
		c.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				BankLatency:      10,
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumSets:          16,
			}).
			Build("Cache")

		// Initialize directoryState
		cache.DirectoryReset(&c.directoryState, 16, 4, 64)

		s = &bankStage{
			cache:           c,
			bankID:          0,
			numReqPerCycle:  1,
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		pipeline.EXPECT().Tick().Return(false)
		inBuf.EXPECT().Peek().Return(nil)
		postPipelineBuf.EXPECT().Peek().Return(nil)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should insert transactions into pipeline", func() {
		trans := &transactionState{}

		inBuf.EXPECT().Peek().Return(trans)
		inBuf.EXPECT().Pop()
		pipeline.EXPECT().Tick().Return(false)
		pipeline.EXPECT().CanAccept().Return(true)
		pipeline.EXPECT().
			Accept(gomock.Any()).
			Do(func(t *bankTransaction) {
				Expect(t.transactionState).To(BeIdenticalTo(trans))
			})
		postPipelineBuf.EXPECT().Peek().Return(nil)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
	})

	Context("read hit", func() {
		var (
			preCRead1, preCRead2, postCRead    *mem.ReadReq
			preCTrans1, preCTrans2, postCTrans *transactionState
			blockSetID, blockWayID             int
		)

		BeforeEach(func() {
			blockSetID = 0
			blockWayID = 0

			// Set up the block in directoryState
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount = 1
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			storage.Write(0x400, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})

			preCRead1 = &mem.ReadReq{}
			preCRead1.ID = sim.GetIDGenerator().Generate()
			preCRead1.Address = 0x104
			preCRead1.AccessByteSize = 4
			preCRead1.TrafficBytes = 12
			preCRead1.TrafficClass = "req"

			preCRead2 = &mem.ReadReq{}
			preCRead2.ID = sim.GetIDGenerator().Generate()
			preCRead2.Address = 0x108
			preCRead2.AccessByteSize = 8
			preCRead2.TrafficBytes = 12
			preCRead2.TrafficClass = "req"

			postCRead = &mem.ReadReq{}
			postCRead.ID = sim.GetIDGenerator().Generate()
			postCRead.Address = 0x100
			postCRead.AccessByteSize = 64
			postCRead.TrafficBytes = 12
			postCRead.TrafficClass = "req"
			preCTrans1 = &transactionState{read: preCRead1}
			preCTrans2 = &transactionState{read: preCRead2}
			postCTrans = &transactionState{
				read:       postCRead,
				blockSetID: blockSetID,
				blockWayID: blockWayID,
				hasBlock:   true,
				bankAction: bankActionReadHit,
				preCoalesceTransactions: []*transactionState{
					preCTrans1, preCTrans2,
				},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)

			postPipelineBuf.EXPECT().Peek().Return(&bankTransaction{
				transactionState: postCTrans,
			})
		})

		It("should read", func() {
			pipeline.EXPECT().Tick()
			inBuf.EXPECT().Peek().Return(nil)
			postPipelineBuf.EXPECT().Pop()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(c.directoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount).To(Equal(0))
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans))
		})
	})

	Context("write", func() {
		var (
			write              *mem.WriteReq
			trans              *transactionState
			blockSetID, blockWayID int
		)

		BeforeEach(func() {
			blockSetID = 0
			blockWayID = 0

			c.directoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x100
			write.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			write.DirtyMask = []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}
			write.TrafficBytes = 64 + 12
			write.TrafficClass = "req"
			trans = &transactionState{
				write:      write,
				blockSetID: blockSetID,
				blockWayID: blockWayID,
				hasBlock:   true,
				bankAction: bankActionWrite,
			}

			postPipelineBuf.EXPECT().
				Peek().
				Return(&bankTransaction{transactionState: trans})
		})

		It("should write", func() {
			pipeline.EXPECT().Tick()
			inBuf.EXPECT().Peek().Return(nil)
			postPipelineBuf.EXPECT().Pop()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked).To(BeFalse())
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal([]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 2, 3, 4, 5, 6, 7, 8,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			}))
		})
	})

	Context("write fetched", func() {
		var (
			trans              *transactionState
			blockSetID, blockWayID int
		)

		BeforeEach(func() {
			blockSetID = 0
			blockWayID = 0

			c.directoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			trans = &transactionState{
				blockSetID: blockSetID,
				blockWayID: blockWayID,
				hasBlock:   true,
				bankAction: bankActionWriteFetched,
			}
			trans.data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			trans.writeFetchedDirtyMask = make([]bool, 64)

			postPipelineBuf.EXPECT().
				Peek().
				Return(&bankTransaction{transactionState: trans})
		})

		It("should write fetched", func() {
			pipeline.EXPECT().Tick()
			inBuf.EXPECT().Peek().Return(nil)
			postPipelineBuf.EXPECT().Pop()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(c.directoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked).To(BeFalse())
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal(trans.data))
		})
	})
})
