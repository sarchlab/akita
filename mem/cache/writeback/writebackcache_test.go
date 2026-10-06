package writeback

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/messaging Port
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/timing Engine

import (
	"log"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"

	"github.com/sarchlab/akita/v5/sim/messaging/direct"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
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
		Top:     twowaybuffered.NewPort(name+".Top", bufSize, bufSize),
		Bottom:  twowaybuffered.NewPort(name+".Bottom", bufSize, bufSize),
		Control: twowaybuffered.NewPort(name+".Control", bufSize, bufSize),
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
	p := twowaybuffered.NewPort(name, bufSize, bufSize)
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
			Top:     twowaybuffered.NewPort("DRAM.Top", 16, 16),
			Control: twowaybuffered.NewPort("DRAM.Control", 16, 16),
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
		conn                *direct.Connection
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

		conn = direct.Definition.Builder().
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

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
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

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x10004,
			Data:    []byte{9, 9, 9, 9}},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len([]byte{9, 9, 9, 9}) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		cacheComp.Ports.Top.Deliver(write)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		Expect(rsp.RspTo).To(Equal(write.ID))

		// Re-read state after engine run
		postState := m.comp.State
		postBlock := &postState.DirectoryState.Sets[setID].Blocks[0]
		retData := m.storage.Read(postBlock.CacheAddress+0x4, 4)
		Expect(retData).To(Equal(write.Payload.(memprotocol.WriteReq).Data))
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

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
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

		read1 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		cacheComp.Ports.Top.Deliver(read1)

		read2 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10008,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		cacheComp.Ports.Top.Deliver(read2)

		Expect(engine.Run()).To(Succeed())

		rsps := map[uint64][]byte{}
		for i := 0; i < 2; i++ {
			rsp, _ := agentPort.RetrieveIncoming()
			Expect(rsp).NotTo(BeNil())
			dr := rsp
			rsps[dr.RspTo] = dr.Payload.(memprotocol.DataReadyRsp).Data
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
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x10000,
			Data:    writeData},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len(writeData) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		cacheComp.Ports.Top.Deliver(write)

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsps := map[uint64]messaging.Msg{}
		for i := 0; i < 2; i++ {
			rsp, _ := agentPort.RetrieveIncoming()
			Expect(rsp).NotTo(BeNil())
			rsps[rsp.RspTo] = rsp
		}
		Expect(rsps).To(HaveKey(write.ID))
		dr := rsps[read.ID]
		Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
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

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		cacheComp.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
		Expect(dr.RspTo).To(Equal(read.ID))
	})

	It("should flush", func() {
		write1 := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100000,
			Data:    []byte{1, 2, 3, 4}},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len([]byte{1, 2, 3, 4}) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		cacheComp.Ports.Top.Deliver(write1)

		write2 := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100000,
			Data:    []byte{1, 2, 3, 4}},
			ID:  m.comp.NewID(),
			Src: agentPort.AsRemote(),
			Dst: cacheComp.Ports.Top.AsRemote(),

			TrafficBytes: len([]byte{1, 2, 3, 4}) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		cacheComp.Ports.Top.Deliver(write2)

		// Let the writes settle so the block is resident and dirty.
		Expect(engine.Run()).To(Succeed())
		_, present0 := controlAgentPort.RetrieveIncoming()
		Expect(present0).To(BeFalse())
		// Flush is a conditional verb: pause first so it is legal.
		pause := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdPause},
			ID:           m.comp.NewID(),
			Src:          controlAgentPort.AsRemote(),
			Dst:          cacheComp.Ports.Control.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		cacheComp.Ports.Control.Deliver(pause)

		Expect(engine.Run()).To(Succeed())

		pauseRsp, _ := controlAgentPort.RetrieveIncoming()
		Expect(pauseRsp).NotTo(BeNil())
		Expect(pauseRsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdPause))
		Expect(pauseRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())

		flush := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
			ID:           m.comp.NewID(),
			Src:          controlAgentPort.AsRemote(),
			Dst:          cacheComp.Ports.Control.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		cacheComp.Ports.Control.Deliver(flush)

		Expect(engine.Run()).To(Succeed())

		rsp, _ := controlAgentPort.RetrieveIncoming()
		Expect(rsp).NotTo(BeNil())
		Expect(rsp.RspTo).To(Equal(flush.ID))
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())

		// The dirty block's data reached DRAM.
		flushed := dramStorage.Read(0x100000, 4)
		Expect(flushed).To(Equal([]byte{1, 2, 3, 4}))
	})
})
