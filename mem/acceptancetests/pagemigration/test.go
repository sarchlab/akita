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
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
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
	l2Cache   *writeback.Comp
	l2TLB     *tlb.Comp
	ioMMU     *mmu.Comp
	memCtrls  []*idealmemcontroller.Comp
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
	rob     *rob.Comp
	at      *addresstranslator.Comp
	l1Cache *writethroughcache.Comp
	l1TLB   *tlb.Comp
}

func setupTest(seed int64) (
	*sim.Simulation,
	timing.Engine,
	[]agentChain,
	sharedHierarchy,
	*directconnection.Comp,
) {
	monitor := monitoring.NewMonitor()
	simBuilder := sim.MakeBuilder().WithMonitor(monitor)

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()
	engine := s.Engine()

	shared := buildSharedHierarchy(s)

	chains := make([]agentChain, *numAgentsFlag)
	for i := 0; i < *numAgentsFlag; i++ {
		chains[i] = buildAgentChain(s, i, shared, seed)
		memaccessagent.CreateProgressBars(chains[i].agent, monitor.CreateProgressBar)
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
func buildSharedHierarchy(s *sim.Simulation) sharedHierarchy {
	combinedRange := uint64(*numAgentsFlag) * agentStride()
	numPages := (combinedRange-1)/pageSize + 1

	// deviceStride must be larger than the largest in-device physical address
	// (ptBase + numPages*pageSize) so that PAddr/deviceStride yields the device
	// index. Round up to a megabyte for readability and margin.
	deviceStride := ptBase + numPages*pageSize
	deviceStride = (deviceStride/mem.MB + 1) * mem.MB

	memCtrls := make([]*idealmemcontroller.Comp, numDevices)
	memCtrlPorts := make([]messaging.RemotePort, numDevices)
	for d := 0; d < numDevices; d++ {
		memCtrls[d] = buildMemCtrl(s, d, uint64(numDevices)*deviceStride+mem.MB)
		memCtrlPorts[d] = memCtrls[d].Ports.Top.AsRemote()
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
	s *sim.Simulation,
	index int,
	capacity uint64,
) *idealmemcontroller.Comp {
	memCtrlSpec := idealmemcontroller.Definition.DefaultSpec
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64

	name := fmt.Sprintf("MemCtrl[%d]", index)
	memCtrl := idealmemcontroller.Definition.Builder().
		WithSimulation(s).
		WithSpec(memCtrlSpec).
		WithResources(idealmemcontroller.Resources{
			Storage: newStorage(s, capacity, name+".Storage"),
		}).
		WithPorts(idealmemcontroller.Ports{
			Top:     newPort(name + ".Top"),
			Control: newPort(name + ".Control"),
		}).
		Build(name)

	return memCtrl
}

// buildL2Cache builds the shared, write-back L2 cache. Its bottom side uses an
// interleaved address mapper so that a physical address routes to the memory
// controller of the device that owns it (address/deviceStride % numDevices).
func buildL2Cache(
	s *sim.Simulation,
	memCtrlPorts []messaging.RemotePort,
	deviceStride uint64,
) *writeback.Comp {
	l2Spec := writeback.Definition.DefaultSpec
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Cache := writeback.Definition.Builder().
		WithSimulation(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			Storage: newStorage(s, l2Spec.TotalByteSize, "L2Cache.Storage"),
			AddressToPortMapper: &mem.InterleavedAddressPortMapper{
				InterleavingSize: deviceStride,
				LowModules:       memCtrlPorts,
			},
		}).
		WithPorts(writeback.Ports{
			Top:     newPort("L2Cache.Top"),
			Bottom:  newPort("L2Cache.Bottom"),
			Control: newPort("L2Cache.Control"),
		}).
		Build("L2Cache")

	return l2Cache
}

func buildMMU(
	s *sim.Simulation,
	pageTable vm.PageTable,
) *mmu.Comp {
	mmuSpec := mmu.Definition.DefaultSpec
	mmuSpec.Log2PageSize = log2PageSize
	mmuSpec.MaxRequestsInFlight = 16
	mmuSpec.Latency = 10
	ioMMU := mmu.Definition.Builder().
		WithSimulation(s).
		WithSpec(mmuSpec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		WithPorts(mmu.Ports{
			Top:     newPort("IoMMU.Top"),
			Control: newPort("IoMMU.Control"),
		}).
		Build("IoMMU")

	return ioMMU
}

func buildL2TLB(
	s *sim.Simulation,
	ioMMU *mmu.Comp,
) *tlb.Comp {
	l2TLBSpec := tlb.Definition.DefaultSpec
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = log2PageSize
	l2TLBSpec.NumReqPerCycle = 4
	l2TLB := tlb.Definition.Builder().
		WithSimulation(s).
		WithSpec(l2TLBSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: ioMMU.Ports.Top.AsRemote(),
			},
		}).
		WithPorts(tlb.Ports{
			Top:     newPort("L2TLB.Top"),
			Bottom:  newPort("L2TLB.Bottom"),
			Control: newPort("L2TLB.Control"),
		}).
		Build("L2TLB")

	return l2TLB
}

// buildAgentChain builds the private ROB, address translator, L1 cache, and
// L1 TLB for one agent, plus the agent itself.
func buildAgentChain(
	s *sim.Simulation,
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
	s *sim.Simulation,
	suffix string,
	l2Cache *writeback.Comp,
) *writethroughcache.Comp {
	l1Spec := writethroughcache.Definition.DefaultSpec
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2
	l1Spec.AddressMapperType = "single"

	name := "L1Cache" + suffix
	l1Cache := writethroughcache.Definition.Builder().
		WithSimulation(s).
		WithSpec(l1Spec).
		WithResources(writethroughcache.Resources{
			Storage: newStorage(s, l1Spec.TotalByteSize, name+".Storage"),
			RemotePorts: []messaging.RemotePort{
				l2Cache.Ports.Top.AsRemote(),
			},
		}).
		WithPorts(writethroughcache.Ports{
			Top:     newPort(name + ".Top"),
			Bottom:  newPort(name + ".Bottom"),
			Control: newPort(name + ".Control"),
		}).
		Build(name)

	return l1Cache
}

func buildL1TLB(
	s *sim.Simulation,
	suffix string,
	l2TLB *tlb.Comp,
) *tlb.Comp {
	l1TLBSpec := tlb.Definition.DefaultSpec
	l1TLBSpec.NumWays = 8
	l1TLBSpec.NumSets = 8
	l1TLBSpec.Log2PageSize = log2PageSize
	l1TLBSpec.NumReqPerCycle = 2

	name := "L1TLB" + suffix
	l1TLB := tlb.Definition.Builder().
		WithSimulation(s).
		WithSpec(l1TLBSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: l2TLB.Ports.Top.AsRemote(),
			},
		}).
		WithPorts(tlb.Ports{
			Top:     newPort(name + ".Top"),
			Bottom:  newPort(name + ".Bottom"),
			Control: newPort(name + ".Control"),
		}).
		Build(name)

	return l1TLB
}

func buildAddressTranslator(
	s *sim.Simulation,
	suffix string,
	l1Cache *writethroughcache.Comp,
	l1TLB *tlb.Comp,
) *addresstranslator.Comp {
	atSpec := addresstranslator.Definition.DefaultSpec
	atSpec.Log2PageSize = log2PageSize
	atSpec.NumReqPerCycle = 4

	name := "AT" + suffix
	at := addresstranslator.Definition.Builder().
		WithSimulation(s).
		WithSpec(atSpec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: l1Cache.Ports.Top.AsRemote(),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: l1TLB.Ports.Top.AsRemote(),
			},
		}).
		WithPorts(addresstranslator.Ports{
			Top:         newPort(name + ".Top"),
			Bottom:      newPort(name + ".Bottom"),
			Translation: newPort(name + ".Translation"),
			Control:     newPort(name + ".Control"),
		}).
		Build(name)

	return at
}

func buildROB(
	s *sim.Simulation,
	suffix string,
	at *addresstranslator.Comp,
) *rob.Comp {
	robSpec := rob.Definition.DefaultSpec
	robSpec.NumReqPerCycle = 4
	robSpec.BottomUnit = at.Ports.Top.AsRemote()

	name := "ROB" + suffix

	return rob.Definition.Builder().
		WithSimulation(s).
		WithSpec(robSpec).
		WithPorts(rob.Ports{
			Top:     newPort(name + ".Top"),
			Bottom:  newPort(name + ".Bottom"),
			Control: newPort(name + ".Control"),
		}).
		Build(name)
}

func buildAgent(
	s *sim.Simulation,
	index int,
	robComp *rob.Comp,
	seed int64,
) *memaccessagent.MemAccessAgent {
	agentSpec := memaccessagent.Definition.DefaultSpec
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.AddressOffset = uint64(index) * agentStride()
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.RandSeed = seed + int64(index)

	name := fmt.Sprintf("MemAccessAgent[%d]", index)
	agent := memaccessagent.Definition.Builder().
		WithSimulation(s).
		WithSpec(agentSpec).
		WithResources(memaccessagent.Resources{
			LowModule: robComp.Ports.Top,
		}).
		WithPorts(memaccessagent.Ports{
			Mem: newPort(name + ".Mem"),
		}).
		Build(name)

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
	s *sim.Simulation,
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
	s *sim.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
) *directconnection.Comp {
	for i, c := range chains {
		suffix := fmt.Sprintf("[%d]", i)

		connect(s, "ConnAgentROB"+suffix,
			c.agent.Ports.Mem,
			c.rob.Ports.Top,
		)
		connect(s, "ConnROBAT"+suffix,
			c.rob.Ports.Bottom,
			c.at.Ports.Top,
		)
		connect(s, "ConnATL1"+suffix,
			c.at.Ports.Bottom,
			c.l1Cache.Ports.Top,
		)
		connect(s, "ConnATTrans"+suffix,
			c.at.Ports.Translation,
			c.l1TLB.Ports.Top,
		)
	}

	// Shared data path: all L1 caches plus the L2 cache on one connection.
	dataConn := directconnection.MakeBuilder().WithSimulation(s).Build("ConnL1L2")
	dataConn.PlugIn(shared.l2Cache.Ports.Top)
	for _, c := range chains {
		dataConn.PlugIn(c.l1Cache.Ports.Bottom)
	}

	// Shared translation path: all L1 TLBs plus the L2 TLB on one connection.
	transConn := directconnection.MakeBuilder().
		WithSimulation(s).
		Build("ConnL1L2TLB")
	transConn.PlugIn(shared.l2TLB.Ports.Top)
	for _, c := range chains {
		transConn.PlugIn(c.l1TLB.Ports.Bottom)
	}

	// L2 cache fans out to every memory controller on one connection; the
	// interleaved mapper picks the right controller per physical address.
	memConn := directconnection.MakeBuilder().WithSimulation(s).Build("ConnL2Mem")
	memConn.PlugIn(shared.l2Cache.Ports.Bottom)
	for _, mc := range shared.memCtrls {
		memConn.PlugIn(mc.Ports.Top)
	}

	connect(s, "ConnL2TLBMMU",
		shared.l2TLB.Ports.Bottom,
		shared.ioMMU.Ports.Top,
	)

	return memConn
}

// newPort creates an unowned port named fullName, for a component that takes
// its ports at Build. The component's Build binds and registers it.
func newPort(fullName string) messaging.Port {
	return messaging.NewPort(fullName, 16, 16)
}

// newStorage builds a storage of the given capacity that registers with the
// simulation.
func newStorage(
	s *sim.Simulation,
	capacity uint64,
	name string,
) *mem.Storage {
	return mem.MakeStorageBuilder().
		WithCapacity(capacity).
		WithSimulation(s).
		Build(name)
}

func connect(s *sim.Simulation, name string, p1, p2 messaging.Port) {
	conn := directconnection.MakeBuilder().WithSimulation(s).Build(name)
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
