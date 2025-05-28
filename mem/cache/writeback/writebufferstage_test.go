package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Write Buffer Stage", func() {
	var (
		mockCtrl            *gomock.Controller
		cacheModule         *Comp
		writeBufferBuffer   *MockBuffer
		bankBuffer          *MockBuffer
		directory           *MockDirectory
		addressToPortMapper *MockAddressToPortMapper
		bottomPort          *MockPort
		mshr                *MockMSHR

		wbStage *writeBufferStage
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		writeBufferBuffer = NewMockBuffer(mockCtrl)
		bankBuffer = NewMockBuffer(mockCtrl)
		directory = NewMockDirectory(mockCtrl)
		directory.EXPECT().WayAssociativity().Return(4).AnyTimes()
		mshr = NewMockMSHR(mockCtrl)
		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()
		builder := MakeBuilder().
			WithAddressToPortMapper(addressToPortMapper)
		cacheModule = builder.Build("Cache")
		cacheModule.bottomPort = bottomPort
		cacheModule.directory = directory
		cacheModule.mshr = mshr
		cacheModule.addressToPortMapper = addressToPortMapper
		cacheModule.writeBufferBuffer = writeBufferBuffer
		cacheModule.writeBufferToBankBuffers = []sim.Buffer{bankBuffer}

		wbStage = &writeBufferStage{
			cache:               cacheModule,
			maxInflightFetch:    64,
			maxInflightEviction: 64,
			writeBufferCapacity: 256,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should reset", func() {
		writeBufferBuffer.EXPECT().Clear()
		wbStage.Reset()
	})

	It("should do nothing if there is no transaction", func() {
		writeBufferBuffer.EXPECT().Peek().Return(nil)

		madeProgress := wbStage.processNewTransaction()

		Expect(madeProgress).To(BeFalse())
	})

	Context("fetch, local hit", func() {
		var (
			eviction  *transaction
			mshrEntry *cache.MSHREntry
			block     *cache.Block
			trans     *transaction
		)

		BeforeEach(func() {
			eviction = &transaction{
				evictingAddr: 0x1000,
				evictingData: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				evictingDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
			}
			wbStage.pendingEvictions = append(
				wbStage.pendingEvictions,
				eviction,
			)

			block = &cache.Block{}
			mshrEntry = cache.NewMSHREntry()
			mshrEntry.Block = block
			trans = &transaction{
				action:       writeBufferFetch,
				block:        block,
				mshrEntry:    mshrEntry,
				fetchAddress: 0x1000,
			}
		})

		It("should stall if bank buffer is full", func() {
			writeBufferBuffer.EXPECT().Peek().Return(trans)
			bankBuffer.EXPECT().CanPush().Return(false)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeFalse())
			Expect(trans.fetchedData).To(BeNil())
			Expect(trans.action).To(Equal(writeBufferFetch))
		})

		It("should do local fetch", func() {
			writeBufferBuffer.EXPECT().Peek().Return(trans)
			writeBufferBuffer.EXPECT().Pop()
			bankBuffer.EXPECT().CanPush().Return(true)
			bankBuffer.EXPECT().Push(trans)
			mshr.EXPECT().Remove(mshrEntry.PID, mshrEntry.Address)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.fetchedData).To(Equal(eviction.evictingData))
			Expect(trans.action).To(Equal(bankWriteFetched))
			Expect(trans.mshrEntry.Data).To(Equal(eviction.evictingData))
		})

		It("should do local fetch if eviction is inflight", func() {
			wbStage.pendingEvictions = nil
			wbStage.inflightEviction = append(
				wbStage.inflightEviction,
				eviction,
			)

			writeBufferBuffer.EXPECT().Peek().Return(trans)
			writeBufferBuffer.EXPECT().Pop()
			bankBuffer.EXPECT().CanPush().Return(true)
			bankBuffer.EXPECT().Push(trans)
			mshr.EXPECT().Remove(mshrEntry.PID, mshrEntry.Address)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.fetchedData).To(Equal(eviction.evictingData))
			Expect(trans.action).To(Equal(bankWriteFetched))
			Expect(trans.mshrEntry.Data).To(Equal(eviction.evictingData))
		})

		It("should combine with write requests", func() {
			write := mem.WriteReqBuilder{}.
				WithAddress(0x204).
				WithData([]byte{10, 10, 10, 10}).
				WithDirtyMask([]bool{true, true, true, true}).
				Build()
			writeTrans := &transaction{write: write}
			trans.mshrEntry.Requests = append(
				trans.mshrEntry.Requests,
				writeTrans,
			)

			wbStage.pendingEvictions = nil
			wbStage.inflightEviction = append(
				wbStage.inflightEviction,
				eviction,
			)

			writeBufferBuffer.EXPECT().Peek().Return(trans)
			writeBufferBuffer.EXPECT().Pop()
			bankBuffer.EXPECT().CanPush().Return(true)
			bankBuffer.EXPECT().Push(trans)
			mshr.EXPECT().Remove(mshrEntry.PID, mshrEntry.Address)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.fetchedData).To(Equal(eviction.evictingData))
			Expect(trans.action).To(Equal(bankWriteFetched))
			Expect(trans.mshrEntry.Data).To(Equal([]byte{
				1, 2, 3, 4, 10, 10, 10, 10,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			Expect(trans.mshrEntry.Block.DirtyMask).To(Equal([]bool{
				false, false, false, false, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}))
		})
	})

	Context("fetch, local miss", func() {
		var (
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.Build()
			trans = &transaction{
				read:         read,
				action:       writeBufferFetch,
				block:        &cache.Block{},
				fetchPID:     1,
				fetchAddress: 0x1000,
			}
			writeBufferBuffer.EXPECT().Peek().Return(trans)
		})

		It("should stall if too many inflight fetch", func() {
			wbStage.inflightFetch = make(
				[]*transaction,
				wbStage.maxInflightFetch,
			)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if cannot send", func() {
			bottomPort.EXPECT().CanSend().Return(false)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send read request to bottom", func() {
			dramPort := NewMockPort(mockCtrl)
			dramPort.EXPECT().
				AsRemote().
				Return(sim.RemotePort("DramPort")).
				AnyTimes()

			var fetchReq *mem.ReadReq

			addressToPortMapper.EXPECT().
				Find(uint64(0x1000)).
				Return(dramPort.AsRemote())
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Do(func(req *mem.ReadReq) {
					fetchReq = req
					Expect(req.Src).To(Equal(cacheModule.bottomPort.AsRemote()))
					Expect(req.Dst).To(Equal(dramPort.AsRemote()))
					Expect(req.PID).To(Equal(trans.fetchPID))
					Expect(req.Address).To(Equal(uint64(0x1000)))
					Expect(req.AccessByteSize).To(Equal(uint64(64)))
				})
			writeBufferBuffer.EXPECT().Pop()

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.fetchReadReq).To(BeIdenticalTo(fetchReq))
			Expect(wbStage.inflightFetch).To(ContainElement(trans))
		})
	})

	Context("evict and write", func() {
		var (
			block *cache.Block
			trans *transaction
		)

		BeforeEach(func() {
			block = &cache.Block{}
			trans = &transaction{
				block:        block,
				action:       writeBufferEvictAndWrite,
				evictingAddr: 0x1000,
				evictingData: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				evictingDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
			}

			writeBufferBuffer.EXPECT().Peek().Return(trans)
		})

		It("should stall if buffer is full", func() {
			wbStage.pendingEvictions = make(
				[]*transaction,
				wbStage.writeBufferCapacity,
			)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeFalse())
			Expect(wbStage.pendingEvictions).NotTo(ContainElement(trans))
		})

		It("should put the new write in write buffer and forward to bank",
			func() {
				writeBufferBuffer.EXPECT().Pop()
				bankBuffer.EXPECT().CanPush().Return(true)
				bankBuffer.EXPECT().
					Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.action).To(Equal(bankWriteHit))
					})

				madeProgress := wbStage.processNewTransaction()

				Expect(madeProgress).To(BeTrue())
				Expect(wbStage.pendingEvictions).To(ContainElement(trans))
			})
	})

	Context("evict", func() {
		var (
			trans *transaction
		)

		BeforeEach(func() {
			trans = &transaction{
				action:       writeBufferFlush,
				evictingAddr: 0x1000,
				evictingData: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				evictingDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
			}

			writeBufferBuffer.EXPECT().Peek().Return(trans)
		})

		It("should stall if buffer is full", func() {
			wbStage.pendingEvictions = make(
				[]*transaction,
				wbStage.writeBufferCapacity,
			)

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeFalse())
			Expect(wbStage.pendingEvictions).NotTo(ContainElement(trans))
		})

		It("should put the new write in write buffer", func() {
			writeBufferBuffer.EXPECT().Pop()

			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeTrue())
			Expect(wbStage.pendingEvictions).To(ContainElement(trans))
		})
	})

	Context("fetch and evict", func() {
		var (
			trans *transaction
		)

		BeforeEach(func() {
			trans = &transaction{
				action:       writeBufferEvictAndFetch,
				fetchAddress: 0x2000,
				evictingAddr: 0x1000,
				evictingData: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				evictingDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
			}

			writeBufferBuffer.EXPECT().Peek().Return(trans)
		})

		It("should first try to evict", func() {
			madeProgress := wbStage.processNewTransaction()

			Expect(madeProgress).To(BeTrue())
			Expect(wbStage.pendingEvictions).To(ContainElement(trans))
			Expect(trans.action).To(Equal(writeBufferFetch))
		})
	})

	Context("when sending write requests", func() {
		var (
			write *mem.WriteReq
			trans *transaction
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.Build()
			trans = &transaction{
				write:        write,
				action:       writeBufferFlush,
				evictingPID:  1,
				evictingAddr: 0x1000,
				evictingData: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				evictingDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
			}

			wbStage.pendingEvictions = append(wbStage.pendingEvictions, trans)
		})

		It("should do nothing if there is nothing to evict", func() {
			wbStage.pendingEvictions = nil

			madeProgress := wbStage.write()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if too many inflight evictions", func() {
			wbStage.inflightEviction = make(
				[]*transaction,
				wbStage.maxInflightEviction,
			)

			madeProgress := wbStage.write()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is buffered sender is full", func() {
			bottomPort.EXPECT().CanSend().Return(false)

			madeProgress := wbStage.write()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send write requests to bottom", func() {
			dramPort := NewMockPort(mockCtrl)
			dramPort.EXPECT().
				AsRemote().
				Return(sim.RemotePort("DramPort")).
				AnyTimes()
			addressToPortMapper.EXPECT().
				Find(uint64(0x1000)).
				Return(dramPort.AsRemote())

			var writeReq *mem.WriteReq
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					writeReq = write
					Expect(write.Src).
						To(Equal(wbStage.cache.bottomPort.AsRemote()))
					Expect(write.Dst).To(Equal(dramPort.AsRemote()))
					Expect(write.PID).To(Equal(trans.evictingPID))
					Expect(write.Address).To(Equal(uint64(0x1000)))
					Expect(write.Data).To(Equal(trans.evictingData))
					Expect(write.DirtyMask).To(Equal(trans.evictingDirtyMask))
				})

			madeProgress := wbStage.write()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.evictionWriteReq).To(BeIdenticalTo(writeReq))
			Expect(wbStage.pendingEvictions).NotTo(ContainElement(trans))
			Expect(wbStage.inflightEviction).To(ContainElement(trans))
		})
	})

	Context("when received write-done rsp", func() {
		var (
			eviction  *transaction
			write     *mem.WriteReq
			writeDone *mem.WriteDoneRsp
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				Build()
			eviction = &transaction{
				evictionWriteReq: write,
			}
			writeDone = mem.WriteDoneRspBuilder{}.
				WithRspTo(write.ID).
				Build()

			wbStage.inflightEviction = append(
				wbStage.inflightEviction,
				eviction,
			)
		})

		It("should do nothing if no return ", func() {
			bottomPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := wbStage.processReturnRsp()

			Expect(madeProgress).To(BeFalse())
		})

		It("should remove inflight eviction", func() {
			bottomPort.EXPECT().PeekIncoming().Return(writeDone)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := wbStage.processReturnRsp()

			Expect(madeProgress).To(BeTrue())
			Expect(wbStage.inflightEviction).NotTo(ContainElement(eviction))
		})
	})

	Context("when received data-ready rsp", func() {
		var (
			read      *mem.ReadReq
			fetch     *transaction
			block     *cache.Block
			mshrEntry *cache.MSHREntry
			dataReady *mem.DataReadyRsp
			data      []byte
		)

		BeforeEach(func() {
			block = &cache.Block{}
			mshrEntry = cache.NewMSHREntry()
			mshrEntry.Block = block

			read = mem.ReadReqBuilder{}.
				WithAddress(0x200).
				Build()
			fetch = &transaction{
				block:        &cache.Block{},
				fetchReadReq: read,
				mshrEntry:    mshrEntry,
			}
			data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			dataReady = mem.DataReadyRspBuilder{}.
				WithRspTo(read.ID).
				WithData(data).
				Build()

			wbStage.inflightFetch = append(wbStage.inflightFetch, fetch)
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
		})

		It("should stall if bank buffer is full", func() {
			bankBuffer.EXPECT().CanPush().Return(false)

			madeProgress := wbStage.processReturnRsp()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send fetched data to bank", func() {
			bankBuffer.EXPECT().CanPush().Return(true)
			bankBuffer.EXPECT().Push(fetch)
			bottomPort.EXPECT().RetrieveIncoming()
			mshr.EXPECT().Remove(mshrEntry.PID, mshrEntry.Address)

			madeProgress := wbStage.processReturnRsp()

			Expect(madeProgress).To(BeTrue())
			Expect(fetch.fetchedData).To(Equal(data))
			Expect(fetch.action).To(Equal(bankWriteFetched))
			Expect(wbStage.inflightFetch).NotTo(ContainElement(fetch))
			Expect(fetch.mshrEntry.Data).To(Equal(data))
		})

		It("should combine with writes in MSHR entry", func() {
			write := mem.WriteReqBuilder{}.
				WithAddress(0x204).
				WithData([]byte{10, 10, 10, 10}).
				WithDirtyMask([]bool{true, true, true, true}).
				Build()
			writeTrans := &transaction{write: write}
			fetch.mshrEntry.Requests = append(
				fetch.mshrEntry.Requests,
				writeTrans,
			)

			bankBuffer.EXPECT().CanPush().Return(true)
			bankBuffer.EXPECT().Push(fetch)
			bottomPort.EXPECT().RetrieveIncoming()
			mshr.EXPECT().Remove(mshrEntry.PID, mshrEntry.Address)

			madeProgress := wbStage.processReturnRsp()

			Expect(madeProgress).To(BeTrue())
			Expect(fetch.fetchedData).To(Equal(data))
			Expect(fetch.action).To(Equal(bankWriteFetched))
			Expect(wbStage.inflightFetch).NotTo(ContainElement(fetch))
			Expect(fetch.mshrEntry.Data).To(Equal([]byte{
				1, 2, 3, 4, 10, 10, 10, 10,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			Expect(fetch.mshrEntry.Block.DirtyMask).To(Equal([]bool{
				false, false, false, false, true, true, true, true,
				false, false, false, false, false, false, false, false,
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
