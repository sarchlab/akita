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
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/akita/v5/messaging"
)

func TestCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Write-Back Suite")
}

// makePorts creates the ports of the writeback cache named name, each with
// bufSize slots in each direction.
func makePorts(name string, bufSize int) Ports {
	return Ports{
		Top:     messaging.NewPort(name+".Top", bufSize, bufSize),
		Bottom:  messaging.NewPort(name+".Bottom", bufSize, bufSize),
		Control: messaging.NewPort(name+".Control", bufSize, bufSize),
	}
}

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(name string, bufSize int) messaging.Port {
	p := messaging.NewPort(name, bufSize, bufSize)
	p.SetOwner(testDriver{})

	return p
}

// plugNoopConn plugs each of comp's ports into a noop connection, so a test
// can drive the ports directly.
func plugNoopConn(comp *Comp) {
	for _, p := range []messaging.Port{
		comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
	} {
		(&ccNoopConn{}).PlugIn(p)
	}
}

// stageTestSpec returns the configuration the white-box stage tests share:
// 64 sets of 4 ways of 64-byte blocks in a single bank, 4 requests per cycle.
func stageTestSpec() Spec {
	spec := Definition.DefaultSpec
	spec.Log2BlockSize = 6
	spec.WayAssociativity = 4
	spec.TotalByteSize = 64 * 4 * 64
	spec.NumBanks = 1
	spec.NumReqPerCycle = 4

	return spec
}

// buildStageTestComp builds the "Cache" instance a white-box stage test
// drives, with its ports plugged into noop connections. The test ticks the
// stage under test directly rather than the component.
func buildStageTestComp(spec Spec, res Resources, ports Ports) *Comp {
	comp := Definition.Builder().
		WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
		WithSpec(spec).
		WithResources(res).
		WithPorts(ports).
		Build("Cache")
	plugNoopConn(comp)

	return comp
}

// buildIdealDRAM builds a 200-cycle ideal memory controller named "DRAM",
// backed by storage, and returns its Top port.
func buildIdealDRAM(sim timing.Simulation, storage *mem.Storage) messaging.Port {
	dramSpec := idealmemcontroller.Definition.DefaultSpec
	dramSpec.Width = 1
	dramSpec.Latency = 200
	dramSpec.CacheLineSize = 64
	dram := idealmemcontroller.Definition.Builder().
		WithSimulation(sim).
		WithResources(idealmemcontroller.Resources{Storage: storage}).
		WithSpec(dramSpec).
		WithPorts(idealmemcontroller.Ports{
			Top:     messaging.NewPort("DRAM.Top", 16, 16),
			Control: messaging.NewPort("DRAM.Control", 16, 16),
		}).
		Build("DRAM")

	return dram.Ports.Top
}

var _ = Describe("Write-Back Cache Integration", func() {
	var (
		engine              timing.Engine
		sim                 timing.Simulation
		addressToPortMapper *mem.SinglePortMapper
		cacheComp           *Comp
		m                   *pipelineMW
		dramTop             messaging.Port
		dramStorage         *mem.Storage
		conn                *directconnection.Comp
		agentPort           messaging.Port
		controlAgentPort    messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		agentPort = newDriverPort("Agent.Top", 8)
		controlAgentPort = newDriverPort("Agent.Control", 8)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dramTop = buildIdealDRAM(sim, dramStorage)

		addressToPortMapper = &mem.SinglePortMapper{
			Port: dramTop.AsRemote(),
		}

		cacheSpec := Definition.DefaultSpec
		cacheSpec.TotalByteSize = 1024 * 4 * 64
		cacheSpec.NumReqPerCycle = 4

		cacheComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(cacheSpec).
			WithResources(Resources{
				Storage:             mem.NewStorage(cacheSpec.TotalByteSize),
				AddressToPortMapper: addressToPortMapper,
			}).
			WithPorts(makePorts("Cache", 8)).
			Build("Cache")
		m = cacheComp.Middlewares.Pipeline

		conn = directconnection.MakeBuilder().
			WithSimulation(sim).
			Build("Connection")
		conn.PlugIn(cacheComp.Ports.Top)
		conn.PlugIn(cacheComp.Ports.Bottom)
		conn.PlugIn(cacheComp.Ports.Control)
		conn.PlugIn(dramTop)
		conn.PlugIn(agentPort)
		conn.PlugIn(controlAgentPort)
	})

	It("should do read hit", func() {
		state := m.comp.State
		spec := m.comp.Spec
		blockSize := 1 << spec.Log2BlockSize
		setID := cache.DirectorySetID(0x10000, blockSize, spec.numSets())
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

		read := memprotocol.ReadReq{}
		read.ID = m.comp.NewID()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.Ports.Top.AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp.(memprotocol.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
		Expect(dr.RspTo).To(Equal(read.ID))
	})

	It("should write hit", func() {
		state := m.comp.State
		spec := m.comp.Spec
		blockSize := 1 << spec.Log2BlockSize
		setID := cache.DirectorySetID(0x10000, blockSize, spec.numSets())
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

		write := memprotocol.WriteReq{}
		write.ID = m.comp.NewID()
		write.Src = agentPort.AsRemote()
		write.Dst = cacheComp.Ports.Top.AsRemote()
		write.Address = 0x10004
		write.Data = []byte{9, 9, 9, 9}
		write.TrafficBytes = len([]byte{9, 9, 9, 9}) + 12
		write.TrafficClass = "memprotocol.WriteReq"
		cacheComp.Ports.Top.Deliver(write)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		Expect(rsp.Meta().RspTo).To(Equal(write.ID))

		// Re-read state after engine run
		postState := m.comp.State
		postBlock := &postState.DirectoryState.Sets[setID].Blocks[0]
		retData := m.storage.Read(postBlock.CacheAddress+0x4, 4)
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

		read := memprotocol.ReadReq{}
		read.ID = m.comp.NewID()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.Ports.Top.AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp.(memprotocol.DataReadyRsp)
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

		read1 := memprotocol.ReadReq{}
		read1.ID = m.comp.NewID()
		read1.Src = agentPort.AsRemote()
		read1.Dst = cacheComp.Ports.Top.AsRemote()
		read1.Address = 0x10004
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "memprotocol.ReadReq"
		cacheComp.Ports.Top.Deliver(read1)

		read2 := memprotocol.ReadReq{}
		read2.ID = m.comp.NewID()
		read2.Src = agentPort.AsRemote()
		read2.Dst = cacheComp.Ports.Top.AsRemote()
		read2.Address = 0x10008
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "memprotocol.ReadReq"
		cacheComp.Ports.Top.Deliver(read2)

		Expect(engine.Run()).To(Succeed())

		rsps := map[uint64][]byte{}
		for i := 0; i < 2; i++ {
			rsp, _ := agentPort.RetrieveIncoming()
			Expect(rsp).NotTo(BeNil())
			dr := rsp.(memprotocol.DataReadyRsp)
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
		write := memprotocol.WriteReq{}
		write.ID = m.comp.NewID()
		write.Src = agentPort.AsRemote()
		write.Dst = cacheComp.Ports.Top.AsRemote()
		write.Address = 0x10000
		write.Data = writeData
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "memprotocol.WriteReq"
		cacheComp.Ports.Top.Deliver(write)

		read := memprotocol.ReadReq{}
		read.ID = m.comp.NewID()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.Ports.Top.AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsps := map[uint64]messaging.Msg{}
		for i := 0; i < 2; i++ {
			rsp, _ := agentPort.RetrieveIncoming()
			Expect(rsp).NotTo(BeNil())
			rsps[rsp.Meta().RspTo] = rsp
		}
		Expect(rsps).To(HaveKey(write.ID))
		dr := rsps[read.ID].(memprotocol.DataReadyRsp)
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
		spec := m.comp.Spec
		blockSize := 1 << spec.Log2BlockSize
		setID := cache.DirectorySetID(0x10000, blockSize, spec.numSets())
		for i := 0; i < spec.WayAssociativity; i++ {
			block := &state.DirectoryState.Sets[setID].Blocks[i]
			block.IsValid = true
			block.IsDirty = true
		}
		m.comp.State = state

		read := memprotocol.ReadReq{}
		read.ID = m.comp.NewID()
		read.Src = agentPort.AsRemote()
		read.Dst = cacheComp.Ports.Top.AsRemote()
		read.Address = 0x10004
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp.(memprotocol.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
		Expect(dr.RspTo).To(Equal(read.ID))
	})

	It("should flush", func() {
		write1 := memprotocol.WriteReq{}
		write1.ID = m.comp.NewID()
		write1.Src = agentPort.AsRemote()
		write1.Dst = cacheComp.Ports.Top.AsRemote()
		write1.Address = 0x100000
		write1.Data = []byte{1, 2, 3, 4}
		write1.TrafficBytes = len([]byte{1, 2, 3, 4}) + 12
		write1.TrafficClass = "memprotocol.WriteReq"
		cacheComp.Ports.Top.Deliver(write1)

		write2 := memprotocol.WriteReq{}
		write2.ID = m.comp.NewID()
		write2.Src = agentPort.AsRemote()
		write2.Dst = cacheComp.Ports.Top.AsRemote()
		write2.Address = 0x100000
		write2.Data = []byte{1, 2, 3, 4}
		write2.TrafficBytes = len([]byte{1, 2, 3, 4}) + 12
		write2.TrafficClass = "memprotocol.WriteReq"
		cacheComp.Ports.Top.Deliver(write2)

		// Let the writes settle so the block is resident and dirty.
		Expect(engine.Run()).To(Succeed())
		_, present0 := controlAgentPort.RetrieveIncoming()
		Expect(present0).To(BeFalse())
		// Flush is a conditional verb: pause first so it is legal.
		pause := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdPause}
		pause.ID = m.comp.NewID()
		pause.Src = controlAgentPort.AsRemote()
		pause.Dst = cacheComp.Ports.Control.AsRemote()
		pause.TrafficClass = "memcontrolprotocol.Req"
		cacheComp.Ports.Control.Deliver(pause)

		Expect(engine.Run()).To(Succeed())

		pauseRsp, _ := controlAgentPort.RetrieveIncoming()
		Expect(pauseRsp).NotTo(BeNil())
		Expect(pauseRsp.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdPause))
		Expect(pauseRsp.(memcontrolprotocol.Rsp).Success).To(BeTrue())

		flush := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush}
		flush.ID = m.comp.NewID()
		flush.Src = controlAgentPort.AsRemote()
		flush.Dst = cacheComp.Ports.Control.AsRemote()
		flush.TrafficClass = "memcontrolprotocol.Req"
		cacheComp.Ports.Control.Deliver(flush)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := controlAgentPort.RetrieveIncoming()
		Expect(rsp).NotTo(BeNil())
		Expect(rsp.Meta().RspTo).To(Equal(flush.ID))
		Expect(rsp.(memcontrolprotocol.Rsp).Success).To(BeTrue())

		// The dirty block's data reached DRAM.
		flushed := dramStorage.Read(0x100000, 4)
		Expect(flushed).To(Equal([]byte{1, 2, 3, 4}))
	})
})
