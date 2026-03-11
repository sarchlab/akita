package writeback

import (
	"log"
	"testing"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port,Engine

func TestCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Write-Back Suite")
}

var _ = Describe("Write-Back Cache Integration", func() {
	var (
		mockCtrl            *gomock.Controller
		engine              sim.Engine
		addressToPortMapper *mem.SinglePortMapper
		cacheComp           *modeling.Component[Spec, State]
		m                   *middleware
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

		dram = idealmemcontroller.MakeBuilder().
			WithEngine(engine).
			WithNewStorage(4 * mem.GB).
			WithFreq(1 * sim.GHz).
			WithSpec(idealmemcontroller.Spec{Width: 1, Latency: 200, CacheLineSize: 64}).
			WithTopPort(sim.NewPort(nil, 16, 16, "DRAM.TopPort")).
			WithCtrlPort(sim.NewPort(nil, 16, 16, "DRAM.CtrlPort")).
			Build("DRAM")

		addressToPortMapper = &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}

		cacheComp = MakeBuilder().
			WithEngine(engine).
			WithByteSize(1024 * 4 * 64).
			WithNumReqPerCycle(4).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 8, 8, "Cache.ToTop")).
			WithBottomPort(sim.NewPort(nil, 8, 8, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 8, 8, "Cache.ControlPort")).
			Build("Cache")
		m = cacheComp.Middlewares()[0].(*middleware)

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Connection")
		conn.PlugIn(cacheComp.GetPortByName("Top"))
		conn.PlugIn(cacheComp.GetPortByName("Bottom"))
		conn.PlugIn(cacheComp.GetPortByName("Control"))
		conn.PlugIn(dram.GetPortByName("Top"))
		conn.PlugIn(agentPort)
		conn.PlugIn(controlAgentPort)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do read hit", func() {
		state := m.comp.GetState()
		spec := m.comp.GetSpec()
		blockSize := 1 << spec.Log2BlockSize
		setID := int(0x10000 / uint64(blockSize) % uint64(spec.NumSets))
		block := &state.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = 0x10000
		block.PID = 0
		block.IsValid = true
		m.comp.SetState(state)
		m.storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RspTo).To(Equal(read.ID))
			})

		engine.Run()
	})

	It("should write hit", func() {
		state := m.comp.GetState()
		spec := m.comp.GetSpec()
		blockSize := 1 << spec.Log2BlockSize
		setID := int(0x10000 / uint64(blockSize) % uint64(spec.NumSets))
		block := &state.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = 0x10000
		block.PID = 0
		block.IsValid = true
		m.comp.SetState(state)
		m.storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Src = agentPort.AsRemote()
		write.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write.Address = 0x10004
		write.Data = []byte{9, 9, 9, 9}
		write.TrafficBytes = len([]byte{9, 9, 9, 9}) + 12
		write.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(write.ID))
			})

		engine.Run()

		// Re-read state after engine run
		postState := m.comp.GetState()
		postBlock := &postState.DirectoryState.Sets[setID].Blocks[0]
		retData, _ := m.storage.Read(postBlock.CacheAddress+0x4, 4)
		Expect(retData).To(Equal(write.Data))
		Expect(postBlock.IsValid).To(BeTrue())
		Expect(postBlock.IsDirty).To(BeTrue())
	})

	It("should do read miss, mshr miss, w/ fetch, w/o eviction", func() {
		dram.GetStorage().Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(msg sim.Msg) {
			dr := msg.(*mem.DataReadyRsp)
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RspTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should handle read miss, mshr hit", func() {
		dram.GetStorage().Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read1 := &mem.ReadReq{}
		read1.ID = sim.GetIDGenerator().Generate()
		read1.Src = agentPort.AsRemote()
		read1.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read1.Address = 0x10004
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read1)

		read2 := &mem.ReadReq{}
		read2.ID = sim.GetIDGenerator().Generate()
		read2.Src = agentPort.AsRemote()
		read2.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read2.Address = 0x10008
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read2)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RspTo).To(Equal(read1.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				Expect(dr.RspTo).To(Equal(read2.ID))
			})

		engine.Run()
	})

	It("should handle write miss, mshr miss, w/o fetch, w/o eviction", func() {
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
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Src = agentPort.AsRemote()
		write.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write.Address = 0x10000
		write.Data = writeData
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write)

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(msg sim.Msg) {
			dr := msg.(*mem.DataReadyRsp)
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RspTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should handle read miss, mshr miss, w/ fetch, w/ eviction", func() {
		dram.GetStorage().Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		// Fill target set with dirty blocks
		state := m.comp.GetState()
		spec := m.comp.GetSpec()
		blockSize := 1 << spec.Log2BlockSize
		setID := int(0x10000 / uint64(blockSize) % uint64(spec.NumSets))
		for i := 0; i < spec.WayAssociativity; i++ {
			block := &state.DirectoryState.Sets[setID].Blocks[i]
			block.IsValid = true
			block.IsDirty = true
		}
		m.comp.SetState(state)

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		agentPort.EXPECT().Deliver(gomock.Any()).Do(func(msg sim.Msg) {
			dr := msg.(*mem.DataReadyRsp)
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RspTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should flush", func() {
		write1 := &mem.WriteReq{}
		write1.ID = sim.GetIDGenerator().Generate()
		write1.Src = agentPort.AsRemote()
		write1.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write1.Address = 0x100000
		write1.Data = []byte{1, 2, 3, 4}
		write1.TrafficBytes = len([]byte{1, 2, 3, 4}) + 12
		write1.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write1)

		write2 := &mem.WriteReq{}
		write2.ID = sim.GetIDGenerator().Generate()
		write2.Src = agentPort.AsRemote()
		write2.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write2.Address = 0x100000
		write2.Data = []byte{1, 2, 3, 4}
		write2.TrafficBytes = len([]byte{1, 2, 3, 4}) + 12
		write2.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write2)

		flush := &cache.FlushReq{}
		flush.ID = sim.GetIDGenerator().Generate()
		flush.Src = controlAgentPort.AsRemote()
		flush.Dst = cacheComp.GetPortByName("Control").AsRemote()
		flush.TrafficClass = "cache.FlushReq"
		cacheComp.GetPortByName("Control").Deliver(flush)

		agentPort.EXPECT().Deliver(gomock.Any()).AnyTimes()

		controlAgentPort.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(flush.ID))
			})

		engine.Run()
	})
})
