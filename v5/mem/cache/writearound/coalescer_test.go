package writearound

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Coalescer", func() {
	var (
		mockCtrl *gomock.Controller
		mw       *middleware
		topPort  *MockPort
		co       coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)

		initialState := State{
			BankBufIndices:             []bankBufState{{Indices: nil}},
			BankPipelineStages:         []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices: []bankPostBufState{{Indices: nil}},
		}

		mw = &middleware{
			topPort: topPort,
		}
		mw.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:         6,
				MaxNumConcurrentTrans: 32,
			}).
			Build("Cache")

		mw.comp.SetState(initialState)

		next := mw.comp.GetNextState()
		mw.dirBufAdapter = &stateTransBuffer{
			name:       "Cache.DirBuf",
			readItems:  &next.DirBufIndices,
			writeItems: &next.DirBufIndices,
			capacity:   4,
			mw:         mw,
		}

		co = coalescer{cache: mw}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no req", func() {
		topPort.EXPECT().PeekIncoming().Return(nil)
		mw.syncForTest()
		madeProgress := co.Tick()
		Expect(madeProgress).To(BeFalse())
	})

	Context("read", func() {
		var (
			read1 *mem.ReadReq
			read2 *mem.ReadReq
		)

		BeforeEach(func() {
			read1 = &mem.ReadReq{}
			read1.ID = sim.GetIDGenerator().Generate()
			read1.Address = 0x100
			read1.PID = 1
			read1.AccessByteSize = 4
			read1.CanWaitForCoalesce = true
			read1.TrafficBytes = 12
			read1.TrafficClass = "req"

			read2 = &mem.ReadReq{}
			read2.ID = sim.GetIDGenerator().Generate()
			read2.Address = 0x104
			read2.PID = 1
			read2.AccessByteSize = 4
			read2.CanWaitForCoalesce = true
			read2.TrafficBytes = 12
			read2.TrafficClass = "req"

			topPort.EXPECT().PeekIncoming().Return(read1)
			topPort.EXPECT().RetrieveIncoming()
			topPort.EXPECT().PeekIncoming().Return(read2)
			topPort.EXPECT().RetrieveIncoming()
			co.Tick()
			co.Tick()
		})

		Context("not coalescable", func() {
			It("should send to dir stage", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.CanWaitForCoalesce = true
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				topPort.EXPECT().PeekIncoming().Return(read3)
				topPort.EXPECT().RetrieveIncoming()

				mw.syncForTest()

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeTrue())
				Expect(mw.transactions).To(HaveLen(3))
				Expect(co.toCoalesce).To(HaveLen(1))
				Expect(mw.postCoalesceTransactions).To(HaveLen(1))
			})

			It("should stall if cannot send to dir", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				mw.dirBufAdapter.capacity = 0

				topPort.EXPECT().PeekIncoming().Return(read3)

				mw.syncForTest()

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(mw.transactions).To(HaveLen(2))
				Expect(co.toCoalesce).To(HaveLen(2))
			})
		})

		Context("last in wave, coalescable", func() {
			It("should send to dir stage", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x108
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				topPort.EXPECT().PeekIncoming().Return(read3)
				topPort.EXPECT().RetrieveIncoming()

				mw.syncForTest()

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeTrue())
				Expect(mw.transactions).To(HaveLen(3))
				Expect(co.toCoalesce).To(HaveLen(0))
				Expect(mw.postCoalesceTransactions).To(HaveLen(1))
				// Check the coalesced transaction
				pt := mw.postCoalesceTransactions[0]
				Expect(pt.preCoalesceTransactions).To(HaveLen(3))
				Expect(pt.read.Address).To(Equal(uint64(0x100)))
				Expect(pt.read.PID).To(Equal(vm.PID(1)))
				Expect(pt.read.AccessByteSize).To(Equal(uint64(64)))
			})

			It("should stall if cannot send", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x108
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				mw.dirBufAdapter.capacity = 0

				topPort.EXPECT().PeekIncoming().Return(read3)

				mw.syncForTest()

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(mw.transactions).To(HaveLen(2))
				Expect(co.toCoalesce).To(HaveLen(2))
			})
		})

		Context("last in wave, not coalescable", func() {
			It("should send to dir stage", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				topPort.EXPECT().PeekIncoming().Return(read3)
				topPort.EXPECT().RetrieveIncoming()

				mw.syncForTest()

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeTrue())
				Expect(mw.transactions).To(HaveLen(3))
				Expect(co.toCoalesce).To(HaveLen(0))
				Expect(mw.postCoalesceTransactions).To(HaveLen(2))
			})

			It("should stall is cannot send to dir stage", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				mw.dirBufAdapter.capacity = 0

				topPort.EXPECT().PeekIncoming().Return(read3)
				mw.syncForTest()
				madeProgress := co.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(mw.transactions).To(HaveLen(2))
				Expect(co.toCoalesce).To(HaveLen(2))
			})

			It("should stall if cannot send to dir stage in the second time",
				func() {
					read3 := &mem.ReadReq{}
					read3.ID = sim.GetIDGenerator().Generate()
					read3.Address = 0x148
					read3.PID = 1
					read3.AccessByteSize = 4
					read3.TrafficBytes = 12
					read3.TrafficClass = "req"

					// Allow first push, then fill buffer
					mw.dirBufAdapter.capacity = 1

					topPort.EXPECT().PeekIncoming().Return(read3)

					mw.syncForTest()

					madeProgress := co.Tick()

					Expect(madeProgress).To(BeTrue())
					Expect(mw.transactions).To(HaveLen(2))
					Expect(co.toCoalesce).To(HaveLen(0))
					Expect(mw.postCoalesceTransactions).To(HaveLen(1))
				})
		})
	})

	Context("write", func() {
		It("should coalesce write", func() {
			write1 := &mem.WriteReq{}
			write1.ID = sim.GetIDGenerator().Generate()
			write1.Address = 0x104
			write1.PID = 1
			write1.Data = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 9, 9, 9}
			write1.DirtyMask = []bool{
				true, true, true, true,
				false, false, false, false,
				true, true, true, true,
			}
			write1.CanWaitForCoalesce = true
			write1.TrafficBytes = 12 + 12
			write1.TrafficClass = "req"

			write2 := &mem.WriteReq{}
			write2.ID = sim.GetIDGenerator().Generate()
			write2.Address = 0x108
			write2.PID = 1
			write2.Data = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 9, 9, 9}
			write2.DirtyMask = []bool{
				true, true, true, true,
				true, true, true, true,
				false, false, false, false,
			}
			write2.TrafficBytes = 12 + 12
			write2.TrafficClass = "req"

			topPort.EXPECT().PeekIncoming().Return(write1)
			topPort.EXPECT().PeekIncoming().Return(write2)
			topPort.EXPECT().RetrieveIncoming().Times(2)

			mw.syncForTest()

			madeProgress := co.Tick()
			Expect(madeProgress).To(BeTrue())

			madeProgress = co.Tick()
			Expect(madeProgress).To(BeTrue())

			Expect(mw.postCoalesceTransactions).To(HaveLen(1))
			pt := mw.postCoalesceTransactions[0]
			Expect(pt.write.Address).To(Equal(uint64(0x100)))
			Expect(pt.write.PID).To(Equal(vm.PID(1)))
			Expect(pt.write.Data).To(Equal([]byte{
				0, 0, 0, 0,
				1, 2, 3, 4,
				1, 2, 3, 4,
				5, 6, 7, 8,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			}))
			Expect(pt.write.DirtyMask).To(Equal([]bool{
				false, false, false, false, true, true, true, true,
				true, true, true, true, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}))
		})
	})
})
