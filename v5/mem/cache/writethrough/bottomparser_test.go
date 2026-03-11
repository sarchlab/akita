package writethrough

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Bottom Parser", func() {
	var (
		mockCtrl   *gomock.Controller
		bottomPort *MockPort
		bankBuf    *MockBuffer
		mshr       *MockMSHR
		p          *bottomParser
		c          *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		c = &Comp{
			log2BlockSize:    6,
			bottomPort:       bottomPort,
			mshr:             mshr,
			wayAssociativity: 4,
			bankBufs:         []queueing.Buffer{bankBuf},
		}
		c.Component = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{}).
			Build("Cache")
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
			write1 := &mem.WriteReq{}
			write1.ID = sim.GetIDGenerator().Generate()
			write1.Address = 0x100
			write1.PID = 1
			write1.TrafficBytes = 12
			write1.TrafficClass = "mem.WriteReq"
			preCTrans1 := &transaction{
				write: write1,
			}
			write2 := &mem.WriteReq{}
			write2.ID = sim.GetIDGenerator().Generate()
			write2.Address = 0x104
			write2.PID = 1
			write2.TrafficBytes = 12
			write2.TrafficClass = "mem.WriteReq"
			preCTrans2 := &transaction{
				write: write2,
			}
			writeToBottom := &mem.WriteReq{}
			writeToBottom.ID = sim.GetIDGenerator().Generate()
			writeToBottom.Address = 0x100
			writeToBottom.PID = 1
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "mem.WriteReq"
			postCTrans := &transaction{
				writeToBottom:           writeToBottom,
				preCoalesceTransactions: []*transaction{preCTrans1, preCTrans2},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)
			done := &mem.WriteDoneRsp{}
			done.ID = sim.GetIDGenerator().Generate()
			done.RspTo = writeToBottom.ID
			done.TrafficBytes = 4
			done.TrafficClass = "mem.WriteDoneRsp"

			bottomPort.EXPECT().PeekIncoming().Return(done)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.done).To(BeTrue())
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans))
		})
	})

	Context("data ready", func() {
		var (
			read1, read2             *mem.ReadReq
			write1, write2           *mem.WriteReq
			preCTrans1, preCTrans2   *transaction
			preCTrans3, preCTrans4   *transaction
			postCRead                *mem.ReadReq
			postCWrite               *mem.WriteReq
			readToBottom             *mem.ReadReq
			block                    *cache.Block
			postCTrans1, postCTrans2 *transaction
			mshrEntry                *cache.MSHREntry
			dataReady                *mem.DataReadyRsp
		)

		BeforeEach(func() {
			read1 = &mem.ReadReq{}
			read1.ID = sim.GetIDGenerator().Generate()
			read1.Address = 0x100
			read1.PID = 1
			read1.AccessByteSize = 4
			read1.TrafficBytes = 12
			read1.TrafficClass = "mem.ReadReq"

			read2 = &mem.ReadReq{}
			read2.ID = sim.GetIDGenerator().Generate()
			read2.Address = 0x104
			read2.PID = 1
			read2.AccessByteSize = 4
			read2.TrafficBytes = 12
			read2.TrafficClass = "mem.ReadReq"

			write1Data := []byte{9, 9, 9, 9}
			write1 = &mem.WriteReq{}
			write1.ID = sim.GetIDGenerator().Generate()
			write1.Address = 0x108
			write1.PID = 1
			write1.Data = write1Data
			write1.TrafficBytes = len(write1Data) + 12
			write1.TrafficClass = "mem.WriteReq"

			write2Data := []byte{9, 9, 9, 9}
			write2 = &mem.WriteReq{}
			write2.ID = sim.GetIDGenerator().Generate()
			write2.Address = 0x10C
			write2.PID = 1
			write2.Data = write2Data
			write2.TrafficBytes = len(write2Data) + 12
			write2.TrafficClass = "mem.WriteReq"

			preCTrans1 = &transaction{read: read1}
			preCTrans2 = &transaction{read: read2}
			preCTrans3 = &transaction{write: write1}
			preCTrans4 = &transaction{write: write2}

			postCRead = &mem.ReadReq{}
			postCRead.ID = sim.GetIDGenerator().Generate()
			postCRead.Address = 0x100
			postCRead.PID = 1
			postCRead.AccessByteSize = 64
			postCRead.TrafficBytes = 12
			postCRead.TrafficClass = "mem.ReadReq"

			readToBottom = &mem.ReadReq{}
			readToBottom.ID = sim.GetIDGenerator().Generate()
			readToBottom.Address = 0x100
			readToBottom.PID = 1
			readToBottom.AccessByteSize = 64
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "mem.ReadReq"

			dataReadyData := []byte{
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
			dataReady.Data = dataReadyData
			dataReady.TrafficBytes = len(dataReadyData) + 4
			dataReady.TrafficClass = "mem.DataReadyRsp"
			block = &cache.Block{
				PID: 1,
				Tag: 0x100,
			}
			postCTrans1 = &transaction{
				block:        block,
				read:         postCRead,
				readToBottom: readToBottom,
				preCoalesceTransactions: []*transaction{
					preCTrans1,
					preCTrans2,
				},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans1)

			postCWriteData := []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				9, 9, 9, 9, 9, 9, 9, 9,
			}
			postCWriteDirtyMask := []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
			}
			postCWrite = &mem.WriteReq{}
			postCWrite.ID = sim.GetIDGenerator().Generate()
			postCWrite.Address = 0x100
			postCWrite.PID = 1
			postCWrite.Data = postCWriteData
			postCWrite.DirtyMask = postCWriteDirtyMask
			postCWrite.TrafficBytes = len(postCWriteData) + 12
			postCWrite.TrafficClass = "mem.WriteReq"
			postCTrans2 = &transaction{
				write: postCWrite,
				preCoalesceTransactions: []*transaction{
					preCTrans3, preCTrans4,
				},
			}

			mshrEntry = &cache.MSHREntry{
				Block: block,
			}
			mshrEntry.Requests = append(mshrEntry.Requests, postCTrans1)
		})

		It("should stall is bank is busy", func() {
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send transaction to bank", func() {
			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			mshr.EXPECT().Remove(vm.PID(1), uint64(0x100))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
				})

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(c.postCoalesceTransactions).
				NotTo(ContainElement(postCTrans1))
		})

		It("should combine write", func() {
			mshrEntry.Requests = append(mshrEntry.Requests, postCTrans2)
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans2)

			bottomPort.EXPECT().PeekIncoming().Return(dataReady)
			bottomPort.EXPECT().RetrieveIncoming()
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			mshr.EXPECT().Remove(vm.PID(1), uint64(0x100))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
					Expect(trans.data).To(Equal([]byte{
						1, 2, 3, 4, 5, 6, 7, 8,
						9, 9, 9, 9, 9, 9, 9, 9,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
					}))
					Expect(trans.writeFetchedDirtyMask).To(Equal([]bool{
						false, false, false, false, false, false, false, false,
						true, true, true, true, true, true, true, true,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
					}))
				})

			madeProgress := p.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans3.done).To(BeTrue())
			Expect(preCTrans4.done).To(BeTrue())
			Expect(c.postCoalesceTransactions).
				NotTo(ContainElement(postCTrans1))
			Expect(c.postCoalesceTransactions).
				NotTo(ContainElement(postCTrans2))
		})
	})

})
