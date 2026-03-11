package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Bankstage", func() {
	var (
		mockCtrl *gomock.Controller
		storage  *mem.Storage
		s        *bankStage
		c        *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		storage = mem.NewStorage(4 * mem.KB)

		initialState := State{
			BankBufIndices:             []bankBufState{{Indices: nil}},
			BankPipelineStages:         []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices: []bankPostBufState{{Indices: nil}},
		}

		c = &middleware{
			storage: storage,
		}
		c.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				BankLatency:      10,
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumSets:          16,
				NumBanks:         1,
				NumReqPerCycle:   1,
			}).
			Build("Cache")

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		c.comp.SetState(initialState)

		next := c.comp.GetNextState()

		// Create adapters
		c.bankBufAdapters = []*stateTransBuffer{
			{
				name:       "Cache.BankBuf0",
				readItems:  &next.BankBufIndices[0].Indices,
				writeItems: &next.BankBufIndices[0].Indices,
				capacity:   1,
				mw:         c,
			},
		}
		c.bankPostBufAdapters = []*stateBankPostBufAdapter{
			{
				name:       "Cache.BankPostBuf0",
				readItems:  &next.BankPostPipelineBufIndices[0].Indices,
				writeItems: &next.BankPostPipelineBufIndices[0].Indices,
				capacity:   1,
				mw:         c,
			},
		}

		s = &bankStage{
			cache:          c,
			bankID:         0,
			numReqPerCycle: 1,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		c.syncForTest()
		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should insert transactions into pipeline", func() {
		next := c.comp.GetNextState()

		trans := &transactionState{}
		c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
		next.BankBufIndices[0].Indices = append(next.BankBufIndices[0].Indices, 0)

		c.syncForTest()

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(next.BankPipelineStages[0].Stages).To(HaveLen(1))
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

			next := c.comp.GetNextState()

			// Set up the block in directoryState
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount = 1
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

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

			// Put in post-pipeline buffer
			next.BankPostPipelineBufIndices[0].Indices = append(
				next.BankPostPipelineBufIndices[0].Indices, 0)
		})

		It("should read", func() {
			next := c.comp.GetNextState()

			c.syncForTest()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount).To(Equal(0))
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

			next := c.comp.GetNextState()

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

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

			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, trans)
			next.BankPostPipelineBufIndices[0].Indices = append(
				next.BankPostPipelineBufIndices[0].Indices, 0)
		})

		It("should write", func() {
			next := c.comp.GetNextState()

			c.syncForTest()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked).To(BeFalse())
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

			next := c.comp.GetNextState()

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

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

			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, trans)
			next.BankPostPipelineBufIndices[0].Indices = append(
				next.BankPostPipelineBufIndices[0].Indices, 0)
		})

		It("should write fetched", func() {
			next := c.comp.GetNextState()

			c.syncForTest()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked).To(BeFalse())
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal(trans.data))
		})
	})
})
