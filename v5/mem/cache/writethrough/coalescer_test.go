package writethrough

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
		cache    *Comp
		topPort  *MockPort
		dirBuf   *MockBuffer
		c        coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		dirBuf = NewMockBuffer(mockCtrl)
		cache = &Comp{
			log2BlockSize:         6,
			topPort:               topPort,
			dirBuf:                dirBuf,
			maxNumConcurrentTrans: 32,
		}
		cache.Component = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{}).
			Build("Cache")
		c = coalescer{cache: cache}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no req", func() {
		topPort.EXPECT().PeekIncoming().Return(nil)
		madeProgress := c.Tick()
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
			read1.TrafficClass = "mem.ReadReq"

			read2 = &mem.ReadReq{}
			read2.ID = sim.GetIDGenerator().Generate()
			read2.Address = 0x104
			read2.PID = 1
			read2.AccessByteSize = 4
			read2.CanWaitForCoalesce = true
			read2.TrafficBytes = 12
			read2.TrafficClass = "mem.ReadReq"

			topPort.EXPECT().PeekIncoming().Return(read1)
			topPort.EXPECT().RetrieveIncoming()
			topPort.EXPECT().PeekIncoming().Return(read2)
			topPort.EXPECT().RetrieveIncoming()
			c.Tick()
			c.Tick()
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
				read3.TrafficClass = "mem.ReadReq"

				dirBuf.EXPECT().CanPush().
					Return(true)
				dirBuf.EXPECT().Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(2))
					})
				topPort.EXPECT().PeekIncoming().Return(read3)
				topPort.EXPECT().RetrieveIncoming()

				madeProgress := c.Tick()

				Expect(madeProgress).To(BeTrue())
				Expect(cache.transactions).To(HaveLen(3))
				Expect(c.toCoalesce).To(HaveLen(1))
				Expect(cache.postCoalesceTransactions).To(HaveLen(1))
			})

			It("should stall if cannot send to dir", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "mem.ReadReq"

				dirBuf.EXPECT().CanPush().
					Return(false)
				topPort.EXPECT().PeekIncoming().Return(read3)

				madeProgress := c.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(cache.transactions).To(HaveLen(2))
				Expect(c.toCoalesce).To(HaveLen(2))
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
				read3.TrafficClass = "mem.ReadReq"

				dirBuf.EXPECT().
					CanPush().
					Return(true)
				dirBuf.EXPECT().
					Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(3))
						Expect(trans.read.Address).To(Equal(uint64(0x100)))
						Expect(trans.read.PID).To(Equal(vm.PID(1)))
						Expect(trans.read.AccessByteSize).To(Equal(uint64(64)))
					})
				topPort.EXPECT().PeekIncoming().Return(read3)
				topPort.EXPECT().RetrieveIncoming()

				madeProgress := c.Tick()

				Expect(madeProgress).To(BeTrue())
				Expect(cache.transactions).To(HaveLen(3))
				Expect(c.toCoalesce).To(HaveLen(0))
				Expect(cache.postCoalesceTransactions).To(HaveLen(1))
			})

			It("should stall if cannot send", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x108
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "mem.ReadReq"

				dirBuf.EXPECT().CanPush().
					Return(false)
				topPort.EXPECT().PeekIncoming().Return(read3)

				madeProgress := c.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(cache.transactions).To(HaveLen(2))
				Expect(c.toCoalesce).To(HaveLen(2))
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
				read3.TrafficClass = "mem.ReadReq"

				dirBuf.EXPECT().CanPush().
					Return(true).Times(2)
				dirBuf.EXPECT().Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(2))
					})
				dirBuf.EXPECT().Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(1))
					})

				topPort.EXPECT().PeekIncoming().Return(read3)
				topPort.EXPECT().RetrieveIncoming()
				madeProgress := c.Tick()

				Expect(madeProgress).To(BeTrue())
				Expect(cache.transactions).To(HaveLen(3))
				Expect(c.toCoalesce).To(HaveLen(0))
				Expect(cache.postCoalesceTransactions).To(HaveLen(2))
			})

			It("should stall is cannot send to dir stage", func() {
				read3 := &mem.ReadReq{}
				read3.ID = sim.GetIDGenerator().Generate()
				read3.Address = 0x148
				read3.PID = 1
				read3.AccessByteSize = 4
				read3.TrafficBytes = 12
				read3.TrafficClass = "mem.ReadReq"

				dirBuf.EXPECT().CanPush().
					Return(false)

				topPort.EXPECT().PeekIncoming().Return(read3)
				madeProgress := c.Tick()

				Expect(madeProgress).To(BeFalse())
				Expect(cache.transactions).To(HaveLen(2))
				Expect(c.toCoalesce).To(HaveLen(2))
			})

			It("should stall if cannot send to dir stage in the second time",
				func() {
					read3 := &mem.ReadReq{}
					read3.ID = sim.GetIDGenerator().Generate()
					read3.Address = 0x148
					read3.PID = 1
					read3.AccessByteSize = 4
					read3.TrafficBytes = 12
					read3.TrafficClass = "mem.ReadReq"

					dirBuf.EXPECT().CanPush().Return(true)
					dirBuf.EXPECT().
						Push(gomock.Any()).
						Do(func(trans *transaction) {
							Expect(trans.preCoalesceTransactions).To(HaveLen(2))
						})
					dirBuf.EXPECT().CanPush().Return(false)
					topPort.EXPECT().PeekIncoming().Return(read3)

					madeProgress := c.Tick()

					Expect(madeProgress).To(BeTrue())
					Expect(cache.transactions).To(HaveLen(2))
					Expect(c.toCoalesce).To(HaveLen(0))
					Expect(cache.postCoalesceTransactions).To(HaveLen(1))
				})
		})
	})

	Context("write", func() {
		It("should coalesce write", func() {
			w1Data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 9, 9, 9}
			write1 := &mem.WriteReq{}
			write1.ID = sim.GetIDGenerator().Generate()
			write1.Address = 0x104
			write1.PID = 1
			write1.Data = w1Data
			write1.DirtyMask = []bool{
				true, true, true, true,
				false, false, false, false,
				true, true, true, true,
			}
			write1.CanWaitForCoalesce = true
			write1.TrafficBytes = len(w1Data) + 12
			write1.TrafficClass = "mem.WriteReq"

			w2Data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 9, 9, 9}
			write2 := &mem.WriteReq{}
			write2.ID = sim.GetIDGenerator().Generate()
			write2.Address = 0x108
			write2.PID = 1
			write2.Data = w2Data
			write2.DirtyMask = []bool{
				true, true, true, true,
				true, true, true, true,
				false, false, false, false,
			}
			write2.TrafficBytes = len(w2Data) + 12
			write2.TrafficClass = "mem.WriteReq"

			topPort.EXPECT().PeekIncoming().Return(write1)
			topPort.EXPECT().PeekIncoming().Return(write2)
			topPort.EXPECT().RetrieveIncoming().Times(2)
			dirBuf.EXPECT().CanPush().Return(true)
			dirBuf.EXPECT().Push(gomock.Any()).Do(func(trans *transaction) {
				Expect(trans.write.Address).To(Equal(uint64(0x100)))
				Expect(trans.write.PID).To(Equal(vm.PID(1)))
				Expect(trans.write.Data).To(Equal([]byte{
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
				Expect(trans.write.DirtyMask).To(Equal([]bool{
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

			madeProgress := c.Tick()
			Expect(madeProgress).To(BeTrue())

			madeProgress = c.Tick()
			Expect(madeProgress).To(BeTrue())

			Expect(cache.postCoalesceTransactions).To(HaveLen(1))
		})
	})
})
