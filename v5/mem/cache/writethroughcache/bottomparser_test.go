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

			write1Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			write2Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Pre-coalesce transactions (indices 0, 1)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    write1Meta,
					WriteAddress: 0x100,
					WritePID:     1,
				},
				transactionState{
					HasWrite:     true,
					WriteMeta:    write2Meta,
					WriteAddress: 0x104,
					WritePID:     1,
				},
			)
			next.NumTransactions = 2

			writeToBottomMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Post-coalesce transaction (index 2, post-coalesce idx 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWriteToBottom:  true,
					WriteToBottomMeta: writeToBottomMeta,
					WriteToBottomPID:  1,
					PreCoalesceTransIdxs: []int{0, 1},
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
			Expect(next.Transactions[1].Done).To(BeTrue())
			// Transaction stays until bankDone is also true.
			postCTrans := next.postCoalesceTrans(0)
			Expect(postCTrans.BottomWriteDone).To(BeTrue())
			Expect(postCTrans.Removed).To(BeFalse())
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

			// Pre-coalesce read transactions (indices 0, 1)
			read1Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			read2Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			// Pre-coalesce write transactions (indices 2, 3)
			write1Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			write2Meta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           read1Meta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
				transactionState{
					HasRead:            true,
					ReadMeta:           read2Meta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
				transactionState{
					HasWrite:     true,
					WriteMeta:    write1Meta,
					WriteAddress: 0x108,
					WriteData:    []byte{9, 9, 9, 9},
					WritePID:     1,
				},
				transactionState{
					HasWrite:     true,
					WriteMeta:    write2Meta,
					WriteAddress: 0x10C,
					WriteData:    []byte{9, 9, 9, 9},
					WritePID:     1,
				},
			)
			next.NumTransactions = 4

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

			postCReadMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Post-coalesce transaction 1 (read, post-coalesce idx 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					BlockSetID:           blockSetID,
					BlockWayID:           blockWayID,
					HasBlock:             true,
					HasRead:              true,
					ReadMeta:             postCReadMeta,
					ReadAddress:          0x100,
					ReadAccessByteSize:   64,
					ReadPID:              1,
					HasReadToBottom:      true,
					ReadToBottomMeta:     readToBottomMeta,
					ReadToBottomPID:      1,
					PreCoalesceTransIdxs: []int{0, 1},
				},
			)

			// Set up MSHR entry with block reference and postCTrans1
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			entry := &next.MSHRState.Entries[entryIdx]
			entry.HasBlock = true
			entry.BlockSetID = blockSetID
			entry.BlockWayID = blockWayID
			entry.TransactionIndices = append(entry.TransactionIndices, 0) // postCTrans1 idx
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
			Expect(next.Transactions[0].Done).To(BeTrue())
			Expect(next.Transactions[0].Data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(next.Transactions[1].Done).To(BeTrue())
			Expect(next.Transactions[1].Data).To(Equal([]byte{5, 6, 7, 8}))
			// Bank buf should have a write-fetched transaction
			Expect(next.BankBufs[0].Elements).To(HaveLen(1))
			// Fetcher trans should have bankAction set
			postCTrans1 := next.postCoalesceTrans(0)
			Expect(postCTrans1.BankAction).To(Equal(bankActionWriteFetched))
		})

		It("should combine write", func() {
			next := c.comp.GetNextState()

			postCWriteMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				TrafficBytes: 16 + 12,
				TrafficClass: "req",
			}

			// Add postCTrans2 as another MSHR request (post-coalesce idx 1)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    postCWriteMeta,
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
					PreCoalesceTransIdxs: []int{2, 3},
				},
			)

			entryIdx, _ := cache.MSHRQuery(&next.MSHRState, vm.PID(1), uint64(0x100))
			next.MSHRState.Entries[entryIdx].TransactionIndices = append(
				next.MSHRState.Entries[entryIdx].TransactionIndices, 1) // postCTrans2 idx

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.Transactions[0].Done).To(BeTrue())
			Expect(next.Transactions[0].Data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(next.Transactions[1].Done).To(BeTrue())
			Expect(next.Transactions[1].Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(next.Transactions[2].Done).To(BeTrue())
			Expect(next.Transactions[3].Done).To(BeTrue())
			// postCTrans1 stays (in bank buf), postCTrans2 is removed
			postCTrans1 := next.postCoalesceTrans(0)
			Expect(postCTrans1.BankAction).To(Equal(bankActionWriteFetched))
			Expect(postCTrans1.Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				9, 9, 9, 9, 9, 9, 9, 9,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			// Bank buf should have the fetcher transaction
			Expect(next.BankBufs[0].Elements).To(HaveLen(1))
		})
	})

})
