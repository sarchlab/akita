package simplecache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Coalescer", func() {
	var (
		mockCtrl *gomock.Controller
		mw       *pipelineMW
		topPort  *MockPort
		co       coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)

		initialState := State{
			DirBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirBuf",
				Cap:        4,
			},
			BankBufs: []stateutil.Buffer[int]{
				{BufferName: "Cache.BankBuf0", Cap: 4},
			},
			DirPipeline: stateutil.Pipeline[int]{
				Width: 4, NumStages: 2,
			},
			DirPostBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirPostBuf",
				Cap:        4,
			},
			BankPipelines: []stateutil.Pipeline[int]{
				{Width: 4, NumStages: 10},
			},
			BankPostBufs: []stateutil.Buffer[int]{
				{BufferName: "Cache.BankPostBuf0", Cap: 4},
			},
		}

		mw = &pipelineMW{
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

		co = coalescer{cache: mw}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no req", func() {
		topPort.EXPECT().PeekIncoming().Return(nil)
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

				madeProgress := co.Tick()

				next := mw.comp.GetNextState()
				Expect(madeProgress).To(BeTrue())
				Expect(next.NumTransactions).To(Equal(3))
				Expect(co.toCoalesce).To(HaveLen(1))
				Expect(next.numPostCoalesce()).To(Equal(1))
			})

			It("should stall if cannot send to dir", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				// Set DirBuf capacity to 0
				next := mw.comp.GetNextState()
				next.DirBuf.Cap = 0

				topPort.EXPECT().PeekIncoming().Return(read3)

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(next.NumTransactions).To(Equal(2))
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

				madeProgress := co.Tick()

				next := mw.comp.GetNextState()
				Expect(madeProgress).To(BeTrue())
				Expect(next.NumTransactions).To(Equal(3))
				Expect(co.toCoalesce).To(HaveLen(0))
				Expect(next.numPostCoalesce()).To(Equal(1))
				// Check the coalesced transaction
				pt := next.postCoalesceTrans(0)
				Expect(pt.PreCoalesceTransIdxs).To(HaveLen(3))
				Expect(pt.ReadAddress).To(Equal(uint64(0x100)))
				Expect(pt.ReadPID).To(Equal(vm.PID(1)))
				Expect(pt.ReadAccessByteSize).To(Equal(uint64(64)))
			})

			It("should stall if cannot send", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x108
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				next := mw.comp.GetNextState()
				next.DirBuf.Cap = 0

				topPort.EXPECT().PeekIncoming().Return(read3)

				madeProgress := co.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(next.NumTransactions).To(Equal(2))
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

				madeProgress := co.Tick()

				next := mw.comp.GetNextState()
				Expect(madeProgress).To(BeTrue())
				Expect(next.NumTransactions).To(Equal(3))
				Expect(co.toCoalesce).To(HaveLen(0))
				Expect(next.numPostCoalesce()).To(Equal(2))
			})

			It("should stall is cannot send to dir stage", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "req"

				next := mw.comp.GetNextState()
				next.DirBuf.Cap = 0

				topPort.EXPECT().PeekIncoming().Return(read3)
				madeProgress := co.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(next.NumTransactions).To(Equal(2))
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
					next := mw.comp.GetNextState()
					next.DirBuf.Cap = 1

					topPort.EXPECT().PeekIncoming().Return(read3)

					madeProgress := co.Tick()

					Expect(madeProgress).To(BeTrue())
					Expect(next.NumTransactions).To(Equal(2))
					Expect(co.toCoalesce).To(HaveLen(0))
					Expect(next.numPostCoalesce()).To(Equal(1))
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

			madeProgress := co.Tick()
			Expect(madeProgress).To(BeTrue())

			madeProgress = co.Tick()
			Expect(madeProgress).To(BeTrue())

			next := mw.comp.GetNextState()
			Expect(next.numPostCoalesce()).To(Equal(1))
			pt := next.postCoalesceTrans(0)
			Expect(pt.WriteAddress).To(Equal(uint64(0x100)))
			Expect(pt.WritePID).To(Equal(vm.PID(1)))
			Expect(pt.WriteData).To(Equal([]byte{
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
			Expect(pt.WriteDirtyMask).To(Equal([]bool{
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
