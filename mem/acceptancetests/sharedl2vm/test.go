// This acceptance test wires four memory-access agents on top of a shared
// lower memory hierarchy. Each agent has its own private ROB, address
// translator, L1 cache, and L1 TLB; all four agents share a single L2 cache,
// L2 TLB, memory controller, and MMU.
//
//	agent_i -Mem-> ROB_i -Bottom-> AT_i
//	                                 |-Bottom------> L1Cache_i --\
//	                                 |-Translation-> L1TLB_i --\  \
//	                                                           |   |  (shared connections)
//	                4x L1TLB.Bottom -----> L2TLB.Top           |   |
//	                                         |                 |   |
//	                                       MMU.Top             |   4x L1Cache.Bottom --> L2Cache.Top
//	                                                           |                            |
//	                                                           +-------------------------> MemCtrl.Top
//
// A directconnection routes by message destination, so a single shared
// connection can fan four private L1s into the one shared L2 (and four L1 TLBs
// into the one L2 TLB): requests carry Dst=L2.Top, responses carry Dst=req.Src
// (the originating L1), so the routing is automatic.
//
// Each agent only verifies the values it itself wrote, so the four agents must
// touch disjoint physical memory. The MemAccessAgent's AddressOffset is set to
// i*MaxAddress for agent i, giving every agent a private slice of the combined
// virtual range [0, numAgents*MaxAddress), which the page table maps to a
// disjoint physical frame.
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

// numAgents is the number of independent memory-access agents.
const numAgents = 4

// ptBase is the physical base address that the page table maps virtual
// address 0 to. Every virtual page is mapped to ptBase + vAddr.
const ptBase = uint64(0x100000)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 1000, "Number of accesses per agent")
var maxAddressFlag = flag.Uint64(
	"max-address", 16*mem.MB, "Per-agent memory range size")

var traceFlag = flag.Bool("trace", false, "Collect trace")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

// sharedHierarchy holds the lower memory components shared by all agents.
type sharedHierarchy struct {
	l2Cache messaging.Component
	l2TLB   messaging.Component
	memCtrl messaging.Component
	ioMMU   messaging.Component
}

// agentChain holds an agent together with its private upstream components.
type agentChain struct {
	agent   *memaccessagent.MemAccessAgent
	rob     messaging.Component
	at      messaging.Component
	l1Cache messaging.Component
	l1TLB   messaging.Component
}

func setupTest(seed int64) (*simulation.Simulation, timing.Engine, []agentChain) {
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

	chains := make([]agentChain, numAgents)
	for i := 0; i < numAgents; i++ {
		chains[i] = buildAgentChain(s, i, shared, seed)
	}

	setupConnections(s, shared, chains)

	return s, engine, chains
}

// agentStride returns the per-agent address-range stride: the configured
// per-agent range rounded up to a 4-byte boundary. Using it for both the agent
// offset (index*stride) and the combined page-table range keeps every agent's
// range disjoint while guaranteeing each agent starts at a 4-byte-aligned
// address, so the agent's 4-byte accesses stay aligned even when -max-address
// is not a multiple of 4. For aligned -max-address (every value the acceptance
// suite uses) the stride equals -max-address, so behavior is unchanged.
func agentStride() uint64 {
	const wordSize = 4
	return (*maxAddressFlag + wordSize - 1) / wordSize * wordSize
}

// buildSharedHierarchy builds the L2 cache, L2 TLB, memory controller, and MMU
// that every agent shares, plus the page table backing the MMU.
func buildSharedHierarchy(s *simulation.Simulation) sharedHierarchy {
	combinedRange := uint64(numAgents) * agentStride()

	memCtrl := buildMemCtrl(s, combinedRange)
	l2Cache := buildL2Cache(s, memCtrl)

	pageTable := setupPageTable(combinedRange, s)
	ioMMU := buildMMU(s, pageTable)
	l2TLB := buildL2TLB(s, ioMMU)

	return sharedHierarchy{
		l2Cache: l2Cache,
		l2TLB:   l2TLB,
		memCtrl: memCtrl,
		ioMMU:   ioMMU,
	}
}

func buildMemCtrl(
	s *simulation.Simulation,
	combinedRange uint64,
) messaging.Component {
	memCtrlSpec := idealmemcontroller.DefaultSpec()
	memCtrlSpec.Capacity = ptBase + combinedRange + mem.MB
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64
	memCtrl := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(memCtrlSpec).
		Build("MemCtrl")
	assignPorts(s, memCtrl, "Top", "Control")

	return memCtrl
}

func buildL2Cache(
	s *simulation.Simulation,
	memCtrl messaging.Component,
) messaging.Component {
	l2Spec := writeback.DefaultSpec()
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Spec.AddressMapperType = "single"
	l2Cache := writeback.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			RemotePorts: []messaging.RemotePort{
				memCtrl.GetPortByName("Top").AsRemote(),
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
	mmuSpec.Log2PageSize = 12
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
	l2TLBSpec.Log2PageSize = 12
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
// L1 TLB for one agent, plus the agent itself. The L1 cache and L1 TLB point
// at the shared L2 cache and L2 TLB respectively.
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
	l1TLBSpec.Log2PageSize = 12
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
	atSpec.Log2PageSize = 12
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

func setupPageTable(addressRange uint64, s *simulation.Simulation) vm.PageTable {
	pageTable := vm.MakePageTableBuilder().
		WithSimulation(s).
		WithLog2PageSize(12).
		Build("PageTable")

	pageSize := uint64(4096)
	numEntries := (addressRange-1)/pageSize + 1

	for i := uint64(0); i < numEntries; i++ {
		vAddr := i * pageSize
		pAddr := ptBase + i*pageSize
		page := vm.Page{
			PID:      1,
			VAddr:    vAddr,
			PAddr:    pAddr,
			PageSize: pageSize,
			Valid:    true,
		}
		pageTable.Insert(page)
	}

	return pageTable
}

// setupConnections wires every private chain and fans the private L1s into the
// shared L2 cache and L2 TLB.
func setupConnections(
	s *simulation.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
) {
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

	connect(s, "ConnL2Mem",
		shared.l2Cache.GetPortByName("Bottom"),
		shared.memCtrl.GetPortByName("Top"),
	)
	connect(s, "ConnL2TLBMMU",
		shared.l2TLB.GetPortByName("Bottom"),
		shared.ioMMU.GetPortByName("Top"),
	)
}

// assignPorts builds a port for each named, declared port of the component
// (with a default buffer size) and assigns it. Every declared port must be
// assigned because the component resolves all of its ports by name on each
// tick.
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

	s, engine, chains := setupTest(seed)

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
		numAgents, *numAccessFlag, *numAccessFlag)

	s.Terminate()
}
