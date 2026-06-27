// This acceptance test models a NUMA-style multi-CPU system and exercises
// transparent page migration. Several memory-access agents (the "CPUs") sit on
// top of a shared memory hierarchy whose physical address space is split across
// two memory devices, each backed by its own memory controller and storage.
//
//	agent_i -Mem-> ROB_i -Bottom-> AT_i
//	                                 |-Bottom------> L1Cache_i --\
//	                                 |-Translation-> L1TLB_i --\  \
//	                                                           |   |  (shared)
//	            4x L1TLB.Bottom -----> L2TLB.Top               |   |
//	                                     |                     |   4x L1Cache.Bottom --> L2Cache.Top
//	                                   MMU.Top                 |                            |
//	                                                           |        L2Cache.Bottom --[interleaved]--> MemCtrl0
//	                                                           |                                      \-> MemCtrl1
//
// NUMA model: there is a single global physical address space. Device d owns the
// address window [d*deviceStride, d*deviceStride + deviceStride). The L2 cache
// routes a physical address to the owning controller with an interleaved mapper
// (address/deviceStride % numDevices), so an access to a page on *any* device
// just works -- a remote access simply routes to the other controller. Migration
// is therefore a transparent performance optimization, not a correctness
// requirement: with migration disabled this test still passes.
//
// Every page has a home slot reserved on *both* devices at the same in-device
// offset, so migrating page i to device d is just PAddr = d*deviceStride +
// ptBase + i*pageSize together with DeviceID = d. The migration controller
// (migrationcontroller.go), enabled with -migrate, periodically relocates pages
// and the agents' value checks are the oracle that proves every migration is
// transparent.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/rob"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

// numDevices is the number of memory devices the physical address space is
// split across.
const numDevices = 2

// pageSize and log2PageSize describe the page granularity used everywhere
// (page table, TLBs, address translators, MMU).
const (
	pageSize     = uint64(4096)
	log2PageSize = uint64(12)
)

// ptBase is the in-device physical offset that the page table maps virtual
// address 0 to. Each device's window starts at d*deviceStride and pages live at
// d*deviceStride + ptBase + vAddr.
const ptBase = uint64(0x100000)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAgentsFlag = flag.Int("num-agents", 2, "Number of memory-access agents")
var numAccessFlag = flag.Int("num-access", 1000, "Number of accesses per agent")
var maxAddressFlag = flag.Uint64(
	"max-address", 16*mem.MB, "Per-agent memory range size")
var migrateFlag = flag.Bool(
	"migrate", true, "Enable the page migration controller")
var migrateIntervalFlag = flag.Uint64(
	"migrate-interval", 2000, "Cycles between migrations")

var traceFlag = flag.Bool("trace", false, "Collect trace")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

// sharedHierarchy holds the lower memory components shared by all agents, plus
// the resources a migration controller needs to relocate pages.
type sharedHierarchy struct {
	l2Cache   messaging.Component
	l2TLB     messaging.Component
	ioMMU     messaging.Component
	memCtrls  []messaging.Component
	pageTable vm.PageTable

	// deviceStride is the size of each device's physical address window and the
	// interleaving size of the L2's address mapper.
	deviceStride uint64
	// numPages is the number of pages in the (shared) virtual address space.
	numPages uint64
}

// agentChain holds an agent together with its private upstream components.
type agentChain struct {
	agent   *memaccessagent.MemAccessAgent
	rob     messaging.Component
	at      messaging.Component
	l1Cache messaging.Component
	l1TLB   messaging.Component
}

func setupTest(seed int64) (
	*simulation.Simulation,
	timing.Engine,
	[]agentChain,
	sharedHierarchy,
	*directconnection.Comp,
) {
	simBuilder := simulation.MakeBuilder()

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()
	engine := s.GetEngine()

	shared := buildSharedHierarchy(s)

	chains := make([]agentChain, *numAgentsFlag)
	for i := 0; i < *numAgentsFlag; i++ {
		chains[i] = buildAgentChain(s, i, shared, seed)
	}

	memConn := setupConnections(s, shared, chains)

	return s, engine, chains, shared, memConn
}

// agentStride returns the per-agent address-range stride: the configured
// per-agent range rounded up to a 4-byte boundary so each agent's range is
// disjoint and word-aligned.
func agentStride() uint64 {
	const wordSize = 4
	return (*maxAddressFlag + wordSize - 1) / wordSize * wordSize
}

// buildSharedHierarchy builds the per-device memory controllers, the shared L2
// cache (with an interleaved mapper that routes physical addresses to the owning
// device), the shared L2 TLB and MMU, and the page table.
func buildSharedHierarchy(s *simulation.Simulation) sharedHierarchy {
	combinedRange := uint64(*numAgentsFlag) * agentStride()
	numPages := (combinedRange-1)/pageSize + 1

	// deviceStride must be larger than the largest in-device physical address
	// (ptBase + numPages*pageSize) so that PAddr/deviceStride yields the device
	// index. Round up to a megabyte for readability and margin.
	deviceStride := ptBase + numPages*pageSize
	deviceStride = (deviceStride/mem.MB + 1) * mem.MB

	memCtrls := make([]messaging.Component, numDevices)
	memCtrlPorts := make([]messaging.RemotePort, numDevices)
	for d := 0; d < numDevices; d++ {
		memCtrls[d] = buildMemCtrl(s, d, uint64(numDevices)*deviceStride+mem.MB)
		memCtrlPorts[d] = memCtrls[d].GetPortByName("Top").AsRemote()
	}

	l2Cache := buildL2Cache(s, memCtrlPorts, deviceStride)

	pageTable := setupPageTable(numPages, deviceStride, s)
	ioMMU := buildMMU(s, pageTable)
	l2TLB := buildL2TLB(s, ioMMU)

	return sharedHierarchy{
		l2Cache:      l2Cache,
		l2TLB:        l2TLB,
		ioMMU:        ioMMU,
		memCtrls:     memCtrls,
		pageTable:    pageTable,
		deviceStride: deviceStride,
		numPages:     numPages,
	}
}

func buildMemCtrl(
	s *simulation.Simulation,
	index int,
	capacity uint64,
) messaging.Component {
	memCtrlSpec := idealmemcontroller.DefaultSpec()
	memCtrlSpec.Capacity = capacity
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64
	memCtrl := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(memCtrlSpec).
		Build(fmt.Sprintf("MemCtrl[%d]", index))
	assignPorts(s, memCtrl, "Top", "Control")

	return memCtrl
}

// buildL2Cache builds the shared, write-back L2 cache. Its bottom side uses an
// interleaved address mapper so that a physical address routes to the memory
// controller of the device that owns it (address/deviceStride % numDevices).
func buildL2Cache(
	s *simulation.Simulation,
	memCtrlPorts []messaging.RemotePort,
	deviceStride uint64,
) messaging.Component {
	l2Spec := writeback.DefaultSpec()
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Cache := writeback.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			AddressToPortMapper: &mem.InterleavedAddressPortMapper{
				InterleavingSize: deviceStride,
				LowModules:       memCtrlPorts,
			},
		}).
		Build("L2Cache")
	assignPorts(s, l2Cache, "Top", "Bottom", "Control")

	return l2Cache
}

func buildMMU(
	s *simulation.Simulation,
	pageTable vm.PageTable,
) messaging.Component {
	mmuSpec := mmu.DefaultSpec()
	mmuSpec.Log2PageSize = log2PageSize
	mmuSpec.MaxRequestsInFlight = 16
	mmuSpec.Latency = 10
	ioMMU := mmu.MakeBuilder().
		WithRegistrar(s).
		WithSpec(mmuSpec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("IoMMU")
	assignPorts(s, ioMMU, "Top", "Control")

	return ioMMU
}

func buildL2TLB(
	s *simulation.Simulation,
	ioMMU messaging.Component,
) messaging.Component {
	l2TLBSpec := tlb.DefaultSpec()
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = log2PageSize
	l2TLBSpec.NumReqPerCycle = 4
	l2TLB := tlb.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2TLBSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: ioMMU.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L2TLB")
	assignPorts(s, l2TLB, "Top", "Bottom", "Control")

	return l2TLB
}

// buildAgentChain builds the private ROB, address translator, L1 cache, and
// L1 TLB for one agent, plus the agent itself.
func buildAgentChain(
	s *simulation.Simulation,
	index int,
	shared sharedHierarchy,
	seed int64,
) agentChain {
	suffix := fmt.Sprintf("[%d]", index)

	l1Cache := buildL1Cache(s, suffix, shared.l2Cache)
	l1TLB := buildL1TLB(s, suffix, shared.l2TLB)
	at := buildAddressTranslator(s, suffix, l1Cache, l1TLB)
	robComp := buildROB(s, suffix, at)
	agent := buildAgent(s, index, robComp, seed)

	return agentChain{
		agent:   agent,
		rob:     robComp,
		at:      at,
		l1Cache: l1Cache,
		l1TLB:   l1TLB,
	}
}

func buildL1Cache(
	s *simulation.Simulation,
	suffix string,
	l2Cache messaging.Component,
) messaging.Component {
	l1Spec := writethroughcache.DefaultSpec()
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2
	l1Spec.AddressMapperType = "single"
	l1Cache := writethroughcache.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l1Spec).
		WithResources(writethroughcache.Resources{
			RemotePorts: []messaging.RemotePort{
				l2Cache.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L1Cache" + suffix)
	assignPorts(s, l1Cache, "Top", "Bottom", "Control")

	return l1Cache
}

func buildL1TLB(
	s *simulation.Simulation,
	suffix string,
	l2TLB messaging.Component,
) messaging.Component {
	l1TLBSpec := tlb.DefaultSpec()
	l1TLBSpec.NumWays = 8
	l1TLBSpec.NumSets = 8
	l1TLBSpec.Log2PageSize = log2PageSize
	l1TLBSpec.NumReqPerCycle = 2
	l1TLB := tlb.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l1TLBSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: l2TLB.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L1TLB" + suffix)
	assignPorts(s, l1TLB, "Top", "Bottom", "Control")

	return l1TLB
}

func buildAddressTranslator(
	s *simulation.Simulation,
	suffix string,
	l1Cache, l1TLB messaging.Component,
) messaging.Component {
	atSpec := addresstranslator.DefaultSpec()
	atSpec.Log2PageSize = log2PageSize
	atSpec.NumReqPerCycle = 4
	at := addresstranslator.MakeBuilder().
		WithRegistrar(s).
		WithSpec(atSpec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: l1Cache.GetPortByName("Top").AsRemote(),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: l1TLB.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("AT" + suffix)
	assignPorts(s, at, "Top", "Bottom", "Translation", "Control")

	return at
}

func buildROB(
	s *simulation.Simulation,
	suffix string,
	at messaging.Component,
) messaging.Component {
	robSpec := rob.DefaultSpec()
	robSpec.NumReqPerCycle = 4
	robSpec.BottomUnit = at.GetPortByName("Top").AsRemote()
	robComp := rob.MakeBuilder().
		WithRegistrar(s).
		WithSpec(robSpec).
		Build("ROB" + suffix)
	assignPorts(s, robComp, "Top", "Bottom", "Control")

	return robComp
}

func buildAgent(
	s *simulation.Simulation,
	index int,
	robComp messaging.Component,
	seed int64,
) *memaccessagent.MemAccessAgent {
	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.AddressOffset = uint64(index) * agentStride()
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.WriteLeft = *numAccessFlag
	agent := memaccessagent.MakeBuilder().
		WithRegistrar(s).
		WithSpec(agentSpec).
		WithRandSeed(seed + int64(index)).
		WithResources(memaccessagent.Resources{
			LowModule: robComp.GetPortByName("Top"),
		}).
		Build(fmt.Sprintf("MemAccessAgent[%d]", index))
	assignPorts(s, agent, "Mem")
	if monitor := s.GetMonitor(); monitor != nil {
		agent.CreateProgressBars(monitor.CreateProgressBar)
	}

	return agent
}

// homeDevice returns the device that initially owns page i.
func homeDevice(pageIndex uint64) uint64 {
	return pageIndex % numDevices
}

// pagePAddr returns the physical address of page i when it lives on device d.
// Each page has a reserved slot at the same in-device offset on every device,
// which makes migration a simple PAddr/DeviceID flip.
func pagePAddr(device, pageIndex, deviceStride uint64) uint64 {
	return device*deviceStride + ptBase + pageIndex*pageSize
}

// setupPageTable builds the shared page table and seeds it with one page per
// virtual page, spreading the pages across the devices so that every agent
// touches both local and remote memory.
func setupPageTable(
	numPages, deviceStride uint64,
	s *simulation.Simulation,
) vm.PageTable {
	pageTable := vm.MakePageTableBuilder().
		WithSimulation(s).
		WithLog2PageSize(log2PageSize).
		Build("PageTable")

	for i := uint64(0); i < numPages; i++ {
		d := homeDevice(i)
		page := vm.Page{
			PID:      1,
			VAddr:    i * pageSize,
			PAddr:    pagePAddr(d, i, deviceStride),
			PageSize: pageSize,
			Valid:    true,
			DeviceID: d,
		}
		pageTable.Insert(page)
	}

	return pageTable
}

// setupConnections wires every private chain and fans the private L1s into the
// shared L2 cache and L2 TLB, and the L2 into the per-device memory controllers.
// It returns the L2<->memory connection so the migration controller can plug
// the data mover's memory-facing ports into the same fabric.
func setupConnections(
	s *simulation.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
) *directconnection.Comp {
	for i, c := range chains {
		suffix := fmt.Sprintf("[%d]", i)

		connect(s, "ConnAgentROB"+suffix,
			c.agent.GetPortByName("Mem"),
			c.rob.GetPortByName("Top"),
		)
		connect(s, "ConnROBAT"+suffix,
			c.rob.GetPortByName("Bottom"),
			c.at.GetPortByName("Top"),
		)
		connect(s, "ConnATL1"+suffix,
			c.at.GetPortByName("Bottom"),
			c.l1Cache.GetPortByName("Top"),
		)
		connect(s, "ConnATTrans"+suffix,
			c.at.GetPortByName("Translation"),
			c.l1TLB.GetPortByName("Top"),
		)
	}

	// Shared data path: all L1 caches plus the L2 cache on one connection.
	dataConn := directconnection.MakeBuilder().WithRegistrar(s).Build("ConnL1L2")
	dataConn.PlugIn(shared.l2Cache.GetPortByName("Top"))
	for _, c := range chains {
		dataConn.PlugIn(c.l1Cache.GetPortByName("Bottom"))
	}

	// Shared translation path: all L1 TLBs plus the L2 TLB on one connection.
	transConn := directconnection.MakeBuilder().
		WithRegistrar(s).
		Build("ConnL1L2TLB")
	transConn.PlugIn(shared.l2TLB.GetPortByName("Top"))
	for _, c := range chains {
		transConn.PlugIn(c.l1TLB.GetPortByName("Bottom"))
	}

	// L2 cache fans out to every memory controller on one connection; the
	// interleaved mapper picks the right controller per physical address.
	memConn := directconnection.MakeBuilder().WithRegistrar(s).Build("ConnL2Mem")
	memConn.PlugIn(shared.l2Cache.GetPortByName("Bottom"))
	for _, mc := range shared.memCtrls {
		memConn.PlugIn(mc.GetPortByName("Top"))
	}

	connect(s, "ConnL2TLBMMU",
		shared.l2TLB.GetPortByName("Bottom"),
		shared.ioMMU.GetPortByName("Top"),
	)

	return memConn
}

// assignPorts builds a port for each named, declared port of the component
// (with a default buffer size) and assigns it.
func assignPorts(
	s *simulation.Simulation,
	comp messaging.Component,
	names ...string,
) {
	for _, name := range names {
		p := modeling.MakePortBuilder().
			WithRegistrar(s).
			WithComponent(comp).
			WithSpec(modeling.PortSpec{BufSize: 16}).
			Build(name)
		comp.AssignPort(name, p)
	}
}

func connect(s *simulation.Simulation, name string, p1, p2 messaging.Port) {
	conn := directconnection.MakeBuilder().WithRegistrar(s).Build(name)
	conn.PlugIn(p1)
	conn.PlugIn(p2)
}

func main() {
	flag.Parse()

	seed := *seedFlag
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)

	s, engine, chains, shared, memConn := setupTest(seed)

	var migCtrl *migrationController
	if *migrateFlag {
		migCtrl = setupMigrationController(s, shared, chains, memConn)
	}

	for _, c := range chains {
		c.agent.TickLater()
	}

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	for i, c := range chains {
		if len(c.agent.State.PendingWriteReq) > 0 ||
			len(c.agent.State.PendingReadReq) > 0 {
			panic(fmt.Sprintf("agent %d: not all req returned", i))
		}

		if c.agent.State.WriteLeft > 0 || c.agent.State.ReadLeft > 0 {
			panic(fmt.Sprintf("agent %d: more requests to send", i))
		}
	}

	fmt.Fprintf(os.Stderr,
		"All %d agents completed %d reads and %d writes each.\n",
		*numAgentsFlag, *numAccessFlag, *numAccessFlag)

	if migCtrl != nil {
		fmt.Fprintf(os.Stderr,
			"Completed %d page migrations.\n", migCtrl.State.NumMigrations)
	}

	s.Terminate()
}
