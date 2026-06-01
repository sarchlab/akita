package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Bankstage", func() {
	var (
		mockCtrl *gomock.Controller
		storage  *mem.Storage
		s        *bankStage
		c        *pipelineMW
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		storage = mem.NewStorage(4 * mem.KB)

		initialState := State{
			DirBuf: queueing.NewBuffer[int]("Cache.DirBuf", 4),
			BankBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.BankBuf0", 1),
			},
			DirPipeline: queueing.NewPipeline[int](1, 2),
			DirPostBuf:  queueing.NewBuffer[int]("Cache.DirPostBuf", 4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.NewPipeline[int](1, 10),
			},
			BankPostBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.BankPostBuf0", 1),
			},
		}

		c = &pipelineMW{
			storage: storage,
		}
		c.comp = modeling.NewBuilder[Spec, State, modeling.None]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				BankLatency:      10,
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumSets:          16,
				NumBanks:         1,
				NumReqPerCycle:   1,
				WritePolicyType:  "write-around",
			}).
			Build("Cache")

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		c.comp.State = initialState

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
		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should insert transactions into pipeline", func() {
		next := &c.comp.State

		// Add a transaction
		next.Transactions = append(next.Transactions, transactionState{})
		next.BankBufs[0].PushTyped(0)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(next.BankPipelines[0].Stages()).To(HaveLen(1))
	})

	Context("read hit", func() {
		var (
			blockSetID, blockWayID int
		)

		BeforeEach(func() {
			blockSetID = 0
			blockWayID = 0

			next := &c.comp.State

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

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					BlockSetID:         blockSetID,
					BlockWayID:         blockWayID,
					HasBlock:           true,
					BankAction:         bankActionReadHit,
				},
			)

			// Put in post-pipeline buffer
			next.BankPostBufs[0].PushTyped(0)
		})

		It("should read", func() {
			next := &c.comp.State

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			trans := &next.Transactions[0]
			Expect(trans.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(trans.Done).To(BeTrue())
			Expect(next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount).To(Equal(0))
		})
	})

	Context("write", func() {
		var (
			blockSetID, blockWayID int
		)

		BeforeEach(func() {
			blockSetID = 0
			blockWayID = 0

			next := &c.comp.State

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				TrafficBytes: 64 + 12,
				TrafficClass: "req",
			}

			writeData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			writeDirtyMask := []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:       true,
					WriteMeta:      writeMeta,
					WriteAddress:   0x100,
					WriteData:      writeData,
					WriteDirtyMask: writeDirtyMask,
					BlockSetID:     blockSetID,
					BlockWayID:     blockWayID,
					HasBlock:       true,
					BankAction:     bankActionWrite,
				},
			)

			next.BankPostBufs[0].PushTyped(0)
		})

		It("should write", func() {
			next := &c.comp.State

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
			blockSetID, blockWayID int
		)

		BeforeEach(func() {
			blockSetID = 0
			blockWayID = 0

			next := &c.comp.State

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			transData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					BlockSetID:            blockSetID,
					BlockWayID:            blockWayID,
					HasBlock:              true,
					BankAction:            bankActionWriteFetched,
					Data:                  transData,
					WriteFetchedDirtyMask: make([]bool, 64),
				},
			)

			next.BankPostBufs[0].PushTyped(0)
		})

		It("should write fetched", func() {
			next := &c.comp.State

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked).To(BeFalse())
			trans := &next.Transactions[0]
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal(trans.Data))
		})
	})
})
