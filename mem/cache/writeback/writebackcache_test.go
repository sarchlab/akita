package writeback

import (
	"log"
	"testing"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE  -write_package_comment=false github.com/sarchlab/akita/v4/mem/cache Directory,MSHR
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE  -write_package_comment=false github.com/sarchlab/akita/v4/mem/mem AddressToPortMapper
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port,Engine,Buffer
//go:generate mockgen -destination "mock_pipelining_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/pipelining Pipeline

func TestCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Write-Back Suite")
}

var _ = Describe("Write-Back Cache Integration", func() {
	var (
		mockCtrl            *gomock.Controller
		engine              sim.Engine
		victimFinder        *cache.LRUVictimFinder
		directory           *cache.DirectoryImpl
		addressToPortMapper *mem.SinglePortMapper
		storage             *mem.Storage
		cacheModule         *Comp
		dram                *idealmemcontroller.Comp
		conn                *directconnection.Comp
		agentPort           *MockPort
		controlAgentPort    *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		agentPort = NewMockPort(mockCtrl)
		agentPort.EXPECT().
			SetConnection(gomock.Any()).
			AnyTimes()
		agentPort.EXPECT().
			PeekOutgoing().
			Return(nil).
			AnyTimes()
		agentPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("AgentPort")).
			AnyTimes()

		controlAgentPort = NewMockPort(mockCtrl)
		controlAgentPort.EXPECT().
			SetConnection(gomock.Any()).
			AnyTimes()
		controlAgentPort.EXPECT().
			PeekOutgoing().
			Return(nil).
			AnyTimes()
		controlAgentPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("ControlAgentPort")).
			AnyTimes()

		engine = sim.NewSerialEngine()
		directory = cache.NewDirectory(1024, 4, 64, victimFinder)
		addressToPortMapper = &mem.SinglePortMapper{}
		storage = mem.NewStorage(1024 * 4 * 64)

		builder := MakeBuilder().
			WithEngine(engine).
			WithByteSize(1024 * 4 * 64).
			WithNumReqPerCycle(4).
			WithAddressToPortMapper(addressToPortMapper)
		cacheModule = builder.Build("Cache")
		cacheModule.directory = directory
		cacheModule.storage = storage

		dram = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithNewStorage(4 * mem.GB).
			WithFreq(1 * sim.GHz).
			WithLatency(200).
			Build("DRAM")

		addressToPortMapper.Port = dram.GetPortByName("Top").AsRemote()

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Connection")
		conn.PlugIn(cacheModule.topPort)
		conn.PlugIn(cacheModule.bottomPort)
		conn.PlugIn(cacheModule.controlPort)
		conn.PlugIn(dram.GetPortByName("Top"))
		conn.PlugIn(agentPort)
		conn.PlugIn(controlAgentPort)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do read hit", func() {
		block := directory.Sets[0].Blocks[0]
		block.Tag = 0x10000
		block.IsValid = true
		storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RespondTo).To(Equal(read.ID))
			})

		engine.Run()

		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should write hit", func() {
		block := directory.Sets[0].Blocks[0]
		block.Tag = 0x10000
		block.IsValid = true
		storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		write := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		cacheModule.topPort.Deliver(write)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		engine.Run()

		retData, _ := storage.Read(0x4, 4)
		Expect(retData).To(Equal(write.Data))
		Expect(block.Tag).To(Equal(uint64(0x10000)))
		Expect(block.IsValid).To(BeTrue())
		Expect(block.IsDirty).To(BeTrue())
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle read miss, mshr hit", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read1 := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read1)

		read2 := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10008).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read2)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RespondTo).To(Equal(read1.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				Expect(dr.RespondTo).To(Equal(read2.ID))
			})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle write miss, mshr hit", func() {
		dram.Storage.Write(0x10000,
			[]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})

		read1 := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read1)

		write := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10008).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		cacheModule.topPort.Deliver(write)

		read2 := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10008).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read2)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RespondTo).To(Equal(read1.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{9, 9, 9, 9}))
				Expect(dr.RespondTo).To(Equal(read2.ID))
			})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should do read miss, mshr miss, w/ fetch, w/o eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should do write miss, mshr miss, w/ fetch, w/o eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		write := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		cacheModule.topPort.Deliver(write)

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10000).
			WithByteSize(8).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})
		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4, 9, 9, 9, 9}))
				Expect(dr.RespondTo).To(Equal(read.ID))
			})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle write miss, mshr miss, w/o fetch, w/o eviction", func() {
		write := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10000).
			WithData([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}).
			Build()
		cacheModule.topPort.Deliver(write)

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()

		retData, _ := storage.Read(0x0, 64)
		Expect(retData).To(Equal(write.Data))
		block := directory.Sets[0].Blocks[0]
		Expect(block.Tag).To(Equal(uint64(0x10000)))
		Expect(block.IsValid).To(BeTrue())
		Expect(block.IsDirty).To(BeTrue())
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle read miss, mshr miss, w/ fetch, w/ eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		set := directory.Sets[0]
		for i := 0; i < directory.WayAssociativity(); i++ {
			set.Blocks[i].IsValid = true
			set.Blocks[i].IsDirty = true
		}

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should handle write miss, mshr miss, w/ fetch, w/ eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		set := directory.Sets[0]
		for i := 0; i < directory.WayAssociativity(); i++ {
			set.Blocks[i].IsValid = true
			set.Blocks[i].IsDirty = true
		}
		write := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10004).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		cacheModule.topPort.Deliver(write)

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10000).
			WithByteSize(8).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4, 9, 9, 9, 9}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should handle write miss, mshr miss, w/ fetch, w/o eviction", func() {
		set := directory.Sets[0]
		for i := 0; i < directory.WayAssociativity(); i++ {
			set.Blocks[i].IsValid = true
			set.Blocks[i].IsDirty = false
		}

		write := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10000).
			WithData([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}).
			Build()
		cacheModule.topPort.Deliver(write)

		read := mem.ReadReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x10000).
			WithByteSize(8).
			Build()
		cacheModule.topPort.Deliver(read)

		agentPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should flush", func() {
		write1 := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x100000).
			WithData([]byte{1, 2, 3, 4}).
			Build()
		cacheModule.topPort.Deliver(write1)

		write2 := mem.WriteReqBuilder{}.
			WithSrc(agentPort.AsRemote()).
			WithDst(cacheModule.topPort.AsRemote()).
			WithAddress(0x100000).
			WithData([]byte{1, 2, 3, 4}).
			Build()
		cacheModule.topPort.Deliver(write2)

		flush := cache.FlushReqBuilder{}.
			WithSrc(controlAgentPort.AsRemote()).
			WithDst(cacheModule.controlPort.AsRemote()).
			Build()
		cacheModule.controlPort.Deliver(flush)

		agentPort.EXPECT().Deliver(gomock.Any()).AnyTimes()

		controlAgentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp *cache.FlushRsp) {
				Expect(rsp.RspTo).To(Equal(flush.ID))
			})

		engine.Run()
	})
})
