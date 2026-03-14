package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Bottom Parser", func() {
	var (
		mockCtrl   *gomock.Controller
		bottomPort *MockPort
		p          *bottomParser
		c          *pipelineMW
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)

		initialState := State{
			DirBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirBuf",
				Cap:        4,
			},
			BankBufs: []queueing.Buffer[int]{
				{BufferName: "Cache.BankBuf0", Cap: 4},
			},
			DirPipeline: queueing.Pipeline[int]{
				Width: 4, NumStages: 2,
			},
			DirPostBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirPostBuf",
				Cap:        4,
			},
			BankPipelines: []queueing.Pipeline[int]{
				{Width: 4, NumStages: 10},
			},
			BankPostBufs: []queueing.Buffer[int]{
				{BufferName: "Cache.BankPostBuf0", Cap: 4},
			},
		}

		c = &pipelineMW{
			bottomPort:  bottomPort,
			writePolicy: &WritearoundPolicy{},
		}
		c.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumMSHREntry:     4,
				NumSets:          16,
				NumBanks:         1,
			}).
			Build("Cache")

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		c.comp.SetState(initialState)

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
			next := c.comp.GetNextState()

			writeToBottomMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:          true,
					WriteMeta:         writeMeta,
					WriteAddress:      0x100,
					WritePID:          1,
					HasWriteToBottom:  true,
					WriteToBottomMeta: writeToBottomMeta,
					WriteToBottomPID:  1,
				},
			)

			done := &mem.WriteDoneRsp{}
			done.ID = sim.GetIDGenerator().Generate()
			done.RspTo = writeToBottomMeta.ID
			done.TrafficBytes = 4
			done.TrafficClass = "rsp"

			bottomPort.EXPECT().PeekIncoming().Return(done)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.Transactions[0].Done).To(BeTrue())
			// Transaction stays until bankDone is also true.
			Expect(next.Transactions[0].BottomWriteDone).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeFalse())
		})
	})

	Context("data ready", func() {
		var (
			readToBottomMeta       sim.MsgMeta
			dataReady              *mem.DataReadyRsp
			blockSetID, blockWayID int
		)

		BeforeEach(func() {
			next := c.comp.GetNextState()

			readToBottomMeta = sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			drData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			dataReady = &mem.DataReadyRsp{}
			dataReady.ID = sim.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottomMeta.ID
			dataReady.Data = drData
			dataReady.TrafficBytes = len(drData) + 4
			dataReady.TrafficClass = "rsp"

			blockSetID = 4
			blockWayID = 0

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].PID = 1
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Read transaction (index 0) — the fetcher
			next.Transactions = append(next.Transactions,
				transactionState{
					BlockSetID:         blockSetID,
					BlockWayID:         blockWayID,
					HasBlock:           true,
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
					HasReadToBottom:    true,
					ReadToBottomMeta:   readToBottomMeta,
					ReadToBottomPID:    1,
				},
			)

			// Set up MSHR entry with block reference and transaction 0
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			entry := &next.MSHRState.Entries[entryIdx]
			entry.HasBlock = true
			entry.BlockSetID = blockSetID
			entry.BlockWayID = blockWayID
			entry.TransactionIndices = append(entry.TransactionIndices, 0)
		})

		It("should stall if bank is busy", func() {
			next := c.comp.GetNextState()
			next.BankBufs[0].Cap = 0

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send transaction to bank", func() {
			next := c.comp.GetNextState()

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			// Fetcher is NOT marked Done yet — bank stage does that
			Expect(next.Transactions[0].Done).To(BeFalse())
			// Fetcher's Data holds the full block for bank write-fetched
			Expect(next.Transactions[0].Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			// Bank buf should have a write-fetched transaction
			Expect(next.BankBufs[0].Elements).To(HaveLen(1))
			// Fetcher trans should have bankAction set
			trans := &next.Transactions[0]
			Expect(trans.BankAction).To(Equal(bankActionWriteFetched))
		})

		It("should combine write", func() {
			next := c.comp.GetNextState()

			// Add another read transaction (index 1) that is in the MSHR
			read2Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           read2Meta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)

			// Add a write transaction (index 2)
			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 16 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x100,
					WritePID:     1,
					WriteData: []byte{
						0, 0, 0, 0, 0, 0, 0, 0,
						9, 9, 9, 9, 9, 9, 9, 9,
					},
					WriteDirtyMask: []bool{
						false, false, false, false, false, false, false, false,
						true, true, true, true, true, true, true, true,
					},
				},
			)

			entryIdx, _ := cache.MSHRQuery(&next.MSHRState, vm.PID(1), uint64(0x100))
			next.MSHRState.Entries[entryIdx].TransactionIndices = append(
				next.MSHRState.Entries[entryIdx].TransactionIndices, 1, 2)

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			// Fetcher's Data holds the full merged block (not the 4-byte
			// read slice — that's restored by the bank stage).
			// Fetcher is NOT marked Done yet — bank stage does that.
			Expect(next.Transactions[0].Done).To(BeFalse())
			trans := &next.Transactions[0]
			Expect(trans.BankAction).To(Equal(bankActionWriteFetched))
			Expect(trans.Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				9, 9, 9, 9, 9, 9, 9, 9,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			// Other transactions finalized
			Expect(next.Transactions[1].Done).To(BeTrue())
			Expect(next.Transactions[1].Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(next.Transactions[2].Done).To(BeTrue())
			// Bank buf should have the fetcher transaction
			Expect(next.BankBufs[0].Elements).To(HaveLen(1))
		})
	})

})
