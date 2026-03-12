package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"

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
			BankBufIndices:             []bankBufState{{Indices: nil}},
			BankPipelineStages:         []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices: []bankPostBufState{{Indices: nil}},
		}

		c = &pipelineMW{
			bottomPort: bottomPort,
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

		next := c.comp.GetNextState()

		// Create adapters
		c.bankBufAdapters = []*stateTransBuffer{
			{
				name:       "Cache.BankBuf0",
				readItems:  &next.BankBufIndices[0].Indices,
				writeItems: &next.BankBufIndices[0].Indices,
				capacity:   4,
				mw:         c,
			},
		}

		p = &bottomParser{cache: c}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no respond", func() {
		bottomPort.EXPECT().PeekIncoming().Return(nil)
		c.syncForTest()
		madeProgress := p.Tick()
		Expect(madeProgress).To(BeFalse())
	})

	Context("write done", func() {
		It("should handle write done", func() {
			write1 := &mem.WriteReq{}
			write1.ID = sim.GetIDGenerator().Generate()
			write1.Address = 0x100
			write1.PID = 1
			write1.TrafficBytes = 12
			write1.TrafficClass = "req"
			preCTrans1 := &transactionState{
				write: write1,
			}
			write2 := &mem.WriteReq{}
			write2.ID = sim.GetIDGenerator().Generate()
			write2.Address = 0x104
			write2.PID = 1
			write2.TrafficBytes = 12
			write2.TrafficClass = "req"
			preCTrans2 := &transactionState{
				write: write2,
			}
			writeToBottom := &mem.WriteReq{}
			writeToBottom.ID = sim.GetIDGenerator().Generate()
			writeToBottom.Address = 0x100
			writeToBottom.PID = 1
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "req"
			postCTrans := &transactionState{
				writeToBottom:           writeToBottom,
				preCoalesceTransactions: []*transactionState{preCTrans1, preCTrans2},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)
			done := &mem.WriteDoneRsp{}
			done.ID = sim.GetIDGenerator().Generate()
			done.RspTo = writeToBottom.ID
			done.TrafficBytes = 4
			done.TrafficClass = "rsp"

			bottomPort.EXPECT().PeekIncoming().Return(done)
			bottomPort.EXPECT().RetrieveIncoming()

			c.syncForTest()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.done).To(BeTrue())
			// Transaction stays until bankDone is also true.
			Expect(postCTrans.bottomWriteDone).To(BeTrue())
			Expect(c.postCoalesceTransactions).To(ContainElement(postCTrans))
		})
	})

	Context("data ready", func() {
		var (
			read1, read2             *mem.ReadReq
			write1, write2           *mem.WriteReq
			preCTrans1, preCTrans2   *transactionState
			preCTrans3, preCTrans4   *transactionState
			postCRead                *mem.ReadReq
			postCWrite               *mem.WriteReq
			readToBottom             *mem.ReadReq
			postCTrans1, postCTrans2 *transactionState
			dataReady                *mem.DataReadyRsp
			blockSetID, blockWayID   int
		)

		BeforeEach(func() {
			read1 = &mem.ReadReq{}
			read1.ID = sim.GetIDGenerator().Generate()
			read1.Address = 0x100
			read1.PID = 1
			read1.AccessByteSize = 4
			read1.TrafficBytes = 12
			read1.TrafficClass = "req"

			read2 = &mem.ReadReq{}
			read2.ID = sim.GetIDGenerator().Generate()
			read2.Address = 0x104
			read2.PID = 1
			read2.AccessByteSize = 4
			read2.TrafficBytes = 12
			read2.TrafficClass = "req"

			write1 = &mem.WriteReq{}
			write1.ID = sim.GetIDGenerator().Generate()
			write1.Address = 0x108
			write1.PID = 1
			write1.Data = []byte{9, 9, 9, 9}
			write1.TrafficBytes = 4 + 12
			write1.TrafficClass = "req"

			write2 = &mem.WriteReq{}
			write2.ID = sim.GetIDGenerator().Generate()
			write2.Address = 0x10C
			write2.PID = 1
			write2.Data = []byte{9, 9, 9, 9}
			write2.TrafficBytes = 4 + 12
			write2.TrafficClass = "req"

			preCTrans1 = &transactionState{read: read1}
			preCTrans2 = &transactionState{read: read2}
			preCTrans3 = &transactionState{write: write1}
			preCTrans4 = &transactionState{write: write2}

			postCRead = &mem.ReadReq{}
			postCRead.ID = sim.GetIDGenerator().Generate()
			postCRead.Address = 0x100
			postCRead.PID = 1
			postCRead.AccessByteSize = 64
			postCRead.TrafficBytes = 12
			postCRead.TrafficClass = "req"

			readToBottom = &mem.ReadReq{}
			readToBottom.ID = sim.GetIDGenerator().Generate()
			readToBottom.Address = 0x100
			readToBottom.PID = 1
			readToBottom.AccessByteSize = 64
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "req"

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
			dataReady.RspTo = readToBottom.ID
			dataReady.Data = drData
			dataReady.TrafficBytes = len(drData) + 4
			dataReady.TrafficClass = "rsp"

			blockSetID = 4
			blockWayID = 0

			next := c.comp.GetNextState()
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].PID = 1
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			postCTrans1 = &transactionState{
				blockSetID:   blockSetID,
				blockWayID:   blockWayID,
				hasBlock:     true,
				read:         postCRead,
				readToBottom: readToBottom,
				preCoalesceTransactions: []*transactionState{
					preCTrans1,
					preCTrans2,
				},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans1)

			postCWrite = &mem.WriteReq{}
			postCWrite.ID = sim.GetIDGenerator().Generate()
			postCWrite.Address = 0x100
			postCWrite.PID = 1
			postCWrite.Data = []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				9, 9, 9, 9, 9, 9, 9, 9,
			}
			postCWrite.DirtyMask = []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
			}
			postCWrite.TrafficBytes = 16 + 12
			postCWrite.TrafficClass = "req"
			postCTrans2 = &transactionState{
				write: postCWrite,
				preCoalesceTransactions: []*transactionState{
					preCTrans3, preCTrans4,
				},
			}

			// Set up MSHR entry with block reference and postCTrans1
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			entry := &next.MSHRState.Entries[entryIdx]
			entry.HasBlock = true
			entry.BlockSetID = blockSetID
			entry.BlockWayID = blockWayID
			entry.TransactionIndices = append(entry.TransactionIndices, 0) // postCTrans1 idx
		})

		It("should stall if bank is busy", func() {
			c.bankBufAdapters[0].capacity = 0

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)

			c.syncForTest()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send transaction to bank", func() {
			next := c.comp.GetNextState()

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()

			c.syncForTest()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			// Bank buf should have a write-fetched transaction
			Expect(next.BankBufIndices[0].Indices).To(HaveLen(1))
			// Fetcher trans should have bankAction set
			Expect(postCTrans1.bankAction).To(Equal(bankActionWriteFetched))
		})

		It("should combine write", func() {
			next := c.comp.GetNextState()

			// Add postCTrans2 as another MSHR request
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans2)
			entryIdx, _ := cache.MSHRQuery(&next.MSHRState, vm.PID(1), uint64(0x100))
			next.MSHRState.Entries[entryIdx].TransactionIndices = append(
				next.MSHRState.Entries[entryIdx].TransactionIndices, 1) // postCTrans2 idx

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()

			c.syncForTest()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans3.done).To(BeTrue())
			Expect(preCTrans4.done).To(BeTrue())
			// postCTrans2 is nil'd out (removed); postCTrans1 stays (in bank buf)
			Expect(postCTrans1.bankAction).To(Equal(bankActionWriteFetched))
			Expect(postCTrans1.data).To(Equal([]byte{
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
			Expect(next.BankBufIndices[0].Indices).To(HaveLen(1))
		})
	})

})
