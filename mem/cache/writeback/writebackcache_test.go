package writeback

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/messaging Port
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/timing Engine

import (
	"log"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/noc/directconnection"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/akita/v5/messaging"
)

func TestCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Write-Back Suite")
}

var _ = Describe("Write-Back Cache Integration", func() {
	var (
		engine              timing.Engine
		addressToPortMapper *mem.SinglePortMapper
		cacheComp           *modeling.Component[Spec, State, Resources]
		m                   *pipelineMW
		dram                *idealmemcontroller.Comp
		dramStorage         *mem.Storage
		conn                *directconnection.Comp
		agentPort           messaging.Port
		controlAgentPort    messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		agentPort = messaging.NewPort(nil, 8, 8, "AgentPort")
		controlAgentPort = messaging.NewPort(nil, 8, 8, "ControlAgentPort")

		dramStorage = mem.NewStorage(4 * mem.GB)
		dramSpec := idealmemcontroller.DefaultSpec()
		dramSpec.Width = 1
		dramSpec.Latency = 200
		dramSpec.CacheLineSize = 64
		dram = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			WithSpec(dramSpec).
			Build("DRAM")
		dram.AssignPort("Top",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Top"))
		dram.AssignPort("Control",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Control"))

		addressToPortMapper = &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}

		cacheSpec := DefaultSpec()
		cacheSpec.TotalByteSize = 1024 * 4 * 64
		cacheSpec.NumReqPerCycle = 4

		cacheComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(cacheSpec).
			WithResources(Resources{
				AddressToPortMapper: addressToPortMapper,
			}).
			Build("Cache")
		// Build only declares the cache's ports; assign the instances and
		// choose their buffer sizes here.
		for _, name := range []string{"Top", "Bottom", "Control"} {
			cacheComp.AssignPort(name,
				messaging.NewPort(cacheComp, 8, 8, cacheComp.Name()+"."+name))
		}
		for _, mw := range cacheComp.Middlewares() {
			if p, ok := mw.(*pipelineMW); ok {
				m = p
			}
		}

		conn = directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Connection")
		conn.PlugIn(cacheComp.GetPortByName("Top"))
		conn.PlugIn(cacheComp.GetPortByName("Bottom"))
		conn.PlugIn(cacheComp.GetPortByName("Control"))
		conn.PlugIn(dram.GetPortByName("Top"))
		conn.PlugIn(agentPort)
		conn.PlugIn(controlAgentPort)
	})

	It("should do read hit", func() {
		state := m.comp.State
		spec := m.comp.Spec()
		blockSize := 1 << spec.Log2BlockSize
		setID := int(0x10000 / uint64(blockSize) % uint64(spec.NumSets))
		block := &state.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = 0x10000
		block.PID = 0
		block.IsValid = true
		m.comp.State = state
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

		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		engine.Run()

		rsp := agentPort.RetrieveIncoming()
		dr := rsp.(mem.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
		Expect(dr.RspTo).To(Equal(read.ID))
	})

	It("should write hit", func() {
		state := m.comp.State
		spec := m.comp.Spec()
		blockSize := 1 << spec.Log2BlockSize
		setID := int(0x10000 / uint64(blockSize) % uint64(spec.NumSets))
		block := &state.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = 0x10000
		block.PID = 0
		block.IsValid = true
		m.comp.State = state
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

		write := mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = agentPort.AsRemote()
		write.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write.Address = 0x10004
		write.Data = []byte{9, 9, 9, 9}
		write.TrafficBytes = len([]byte{9, 9, 9, 9}) + 12
		write.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write)

		engine.Run()

		rsp := agentPort.RetrieveIncoming()
		Expect(rsp.Meta().RspTo).To(Equal(write.ID))

		// Re-read state after engine run
		postState := m.comp.State
		postBlock := &postState.DirectoryState.Sets[setID].Blocks[0]
		retData, _ := m.storage.Read(postBlock.CacheAddress+0x4, 4)
		Expect(retData).To(Equal(write.Data))
		Expect(postBlock.IsValid).To(BeTrue())
		Expect(postBlock.IsDirty).To(BeTrue())
	})

	It("should do read miss, mshr miss, w/ fetch, w/o eviction", func() {
		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		engine.Run()

		rsp := agentPort.RetrieveIncoming()
		dr := rsp.(mem.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
		Expect(dr.RspTo).To(Equal(read.ID))
	})

	It("should handle read miss, mshr hit", func() {
		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read1 := mem.ReadReq{}
		read1.ID = timing.GetIDGenerator().Generate()
		read1.Src = agentPort.AsRemote()
		read1.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read1.Address = 0x10004
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read1)

		read2 := mem.ReadReq{}
		read2.ID = timing.GetIDGenerator().Generate()
		read2.Src = agentPort.AsRemote()
		read2.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read2.Address = 0x10008
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read2)

		engine.Run()

		rsps := map[uint64][]byte{}
		for i := 0; i < 2; i++ {
			rsp := agentPort.RetrieveIncoming()
			Expect(rsp).NotTo(BeNil())
			dr := rsp.(mem.DataReadyRsp)
			rsps[dr.RspTo] = dr.Data
		}
		Expect(rsps[read1.ID]).To(Equal([]byte{5, 6, 7, 8}))
		Expect(rsps[read2.ID]).To(Equal([]byte{1, 2, 3, 4}))
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
		write := mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = agentPort.AsRemote()
		write.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write.Address = 0x10000
		write.Data = writeData
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write)

		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		engine.Run()

		rsps := map[uint64]messaging.Msg{}
		for i := 0; i < 2; i++ {
			rsp := agentPort.RetrieveIncoming()
			Expect(rsp).NotTo(BeNil())
			rsps[rsp.Meta().RspTo] = rsp
		}
		Expect(rsps).To(HaveKey(write.ID))
		dr := rsps[read.ID].(mem.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
	})

	It("should handle read miss, mshr miss, w/ fetch, w/ eviction", func() {
		dramStorage.Write(0x10000, []byte{
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
		state := m.comp.State
		spec := m.comp.Spec()
		blockSize := 1 << spec.Log2BlockSize
		setID := int(0x10000 / uint64(blockSize) % uint64(spec.NumSets))
		for i := 0; i < spec.WayAssociativity; i++ {
			block := &state.DirectoryState.Sets[setID].Blocks[i]
			block.IsValid = true
			block.IsDirty = true
		}
		m.comp.State = state

		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.GetPortByName("Top").AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		cacheComp.GetPortByName("Top").Deliver(read)

		engine.Run()

		rsp := agentPort.RetrieveIncoming()
		dr := rsp.(mem.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
		Expect(dr.RspTo).To(Equal(read.ID))
	})

	It("should flush", func() {
		write1 := mem.WriteReq{}
		write1.ID = timing.GetIDGenerator().Generate()
		write1.Src = agentPort.AsRemote()
		write1.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write1.Address = 0x100000
		write1.Data = []byte{1, 2, 3, 4}
		write1.TrafficBytes = len([]byte{1, 2, 3, 4}) + 12
		write1.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write1)

		write2 := mem.WriteReq{}
		write2.ID = timing.GetIDGenerator().Generate()
		write2.Src = agentPort.AsRemote()
		write2.Dst = cacheComp.GetPortByName("Top").AsRemote()
		write2.Address = 0x100000
		write2.Data = []byte{1, 2, 3, 4}
		write2.TrafficBytes = len([]byte{1, 2, 3, 4}) + 12
		write2.TrafficClass = "mem.WriteReq"
		cacheComp.GetPortByName("Top").Deliver(write2)

		// Let the writes settle so the block is resident and dirty.
		engine.Run()
		Expect(controlAgentPort.RetrieveIncoming()).To(BeNil())

		// Flush is a conditional verb: pause first so it is legal.
		pause := mem.ControlReq{Command: mem.CmdPause}
		pause.ID = timing.GetIDGenerator().Generate()
		pause.Src = controlAgentPort.AsRemote()
		pause.Dst = cacheComp.GetPortByName("Control").AsRemote()
		pause.TrafficClass = "mem.ControlReq"
		cacheComp.GetPortByName("Control").Deliver(pause)

		engine.Run()

		pauseRsp := controlAgentPort.RetrieveIncoming()
		Expect(pauseRsp).NotTo(BeNil())
		Expect(pauseRsp.(mem.ControlRsp).Command).To(Equal(mem.CmdPause))
		Expect(pauseRsp.(mem.ControlRsp).Success).To(BeTrue())

		flush := mem.ControlReq{Command: mem.CmdFlush}
		flush.ID = timing.GetIDGenerator().Generate()
		flush.Src = controlAgentPort.AsRemote()
		flush.Dst = cacheComp.GetPortByName("Control").AsRemote()
		flush.TrafficClass = "mem.ControlReq"
		cacheComp.GetPortByName("Control").Deliver(flush)

		engine.Run()

		rsp := controlAgentPort.RetrieveIncoming()
		Expect(rsp).NotTo(BeNil())
		Expect(rsp.Meta().RspTo).To(Equal(flush.ID))
		Expect(rsp.(mem.ControlRsp).Success).To(BeTrue())

		// The dirty block's data reached DRAM.
		flushed, err := dramStorage.Read(0x100000, 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(flushed).To(Equal([]byte{1, 2, 3, 4}))
	})
})
