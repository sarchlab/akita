package main

import (
	"flag"
	"fmt"
	"math/rand"
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

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 10000, "Number of accesses")
var maxAddressFlag = flag.Uint64("max-address", 1*mem.GB, "Max memory address")

var traceFlag = flag.Bool("trace", false, "Collect trace")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

var agent *memaccessagent.MemAccessAgent

//nolint:funlen // wires the whole simulation in one place
func setupTest(
	seed int64,
) (*sim.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
	var monitor *monitoring.Monitor
	simBuilder := sim.MakeBuilder().WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
		monitor = monitoring.NewMonitor(s)
		return monitor
	})

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()

	engine := s.Engine()

	l1Cache, l2Cache, memCtrl := buildMemoryHierarchy(s)
	ioMMU, tlb, l2TLB := buildTranslationHierarchy(s)

	atMemoryMapper := &mem.SinglePortMapper{
		Port: l1Cache.Ports.Top.AsRemote(),
	}
	atTranslationMapper := &mem.SinglePortMapper{
		Port: tlb.Ports.Top.AsRemote(),
	}

	atSpec := addresstranslator.Definition.DefaultSpec
	atSpec.Log2PageSize = 12
	atSpec.NumReqPerCycle = 4
	at := addresstranslator.Definition.Builder().
		WithSimulation(s).
		WithSpec(atSpec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper:         atMemoryMapper,
			TranslationProviderMapper: atTranslationMapper,
		}).
		WithPorts(addresstranslator.Ports{
			Top:         newPort("AT.Top"),
			Bottom:      newPort("AT.Bottom"),
			Translation: newPort("AT.Translation"),
			Control:     newPort("AT.Control"),
		}).
		Build("AT")

	// Insert a reorder buffer between the agent and the address translator so
	// the trace exercises the ROB instrumentation end-to-end.
	robComp := buildROB(s, at.Ports.Top.AsRemote())

	agentSpec := memaccessagent.Definition.DefaultSpec
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.RandSeed = seed
	agent = memaccessagent.Definition.Builder().
		WithSimulation(s).
		WithSpec(agentSpec).
		WithResources(memaccessagent.Resources{
			LowModule: robComp.Ports.Top,
		}).
		WithPorts(memaccessagent.Ports{
			Mem: newPort("MemAccessAgent.Mem"),
		}).
		Build("MemAccessAgent")
	memaccessagent.CreateProgressBars(agent, monitor.CreateProgressBar)

	setupConnection(s, agent, robComp,
		at, tlb, l2TLB, ioMMU,
		l1Cache, l2Cache, memCtrl)

	return s, engine, agent
}

// buildROB builds a reorder buffer that forwards every access to bottomUnit
// (the address translator's Top port) and reorders the responses back.
func buildROB(s *sim.Simulation, bottomUnit messaging.RemotePort) *rob.Comp {
	robSpec := rob.Definition.DefaultSpec
	robSpec.NumReqPerCycle = 4
	robSpec.BottomUnit = bottomUnit

	return rob.Definition.Builder().
		WithSimulation(s).
		WithSpec(robSpec).
		WithPorts(rob.Ports{
			Top:     newPort("ROB.Top"),
			Bottom:  newPort("ROB.Bottom"),
			Control: newPort("ROB.Control"),
		}).
		Build("ROB")
}

//nolint:funlen // wires the whole hierarchy in one place
func buildMemoryHierarchy(s *sim.Simulation) (
	*writethroughcache.Comp,
	*writeback.Comp,
	*idealmemcontroller.Comp,
) {
	memCtrlSpec := idealmemcontroller.Definition.DefaultSpec
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64
	memCtrl := idealmemcontroller.Definition.Builder().
		WithSimulation(s).
		WithSpec(memCtrlSpec).
		WithResources(idealmemcontroller.Resources{
			Storage: newStorage(s, 4*mem.GB, "MemCtrl.Storage"),
		}).
		WithPorts(idealmemcontroller.Ports{
			Top:     newPort("MemCtrl.Top"),
			Control: newPort("MemCtrl.Control"),
		}).
		Build("MemCtrl")

	l2Spec := writeback.Definition.DefaultSpec
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Spec.AddressMapperType = "single"
	L2Cache := writeback.Definition.Builder().
		WithSimulation(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			Storage: newStorage(s, l2Spec.TotalByteSize, "L2Cache.Storage"),
			RemotePorts: []messaging.RemotePort{
				memCtrl.Ports.Top.AsRemote(),
			},
		}).
		WithPorts(writeback.Ports{
			Top:     newPort("L2Cache.Top"),
			Bottom:  newPort("L2Cache.Bottom"),
			Control: newPort("L2Cache.Control"),
		}).
		Build("L2Cache")

	l1Spec := writethroughcache.Definition.DefaultSpec
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2
	l1Spec.AddressMapperType = "single"
	L1Cache := writethroughcache.Definition.Builder().
		WithSimulation(s).
		WithSpec(l1Spec).
		WithResources(writethroughcache.Resources{
			Storage: newStorage(s, l1Spec.TotalByteSize, "L1Cache.Storage"),
			RemotePorts: []messaging.RemotePort{
				L2Cache.Ports.Top.AsRemote(),
			},
		}).
		WithPorts(writethroughcache.Ports{
			Top:     newPort("L1Cache.Top"),
			Bottom:  newPort("L1Cache.Bottom"),
			Control: newPort("L1Cache.Control"),
		}).
		Build("L1Cache")

	return L1Cache, L2Cache, memCtrl
}

//nolint:funlen // wires the whole hierarchy in one place
func buildTranslationHierarchy(
	s *sim.Simulation,
) (
	*mmu.Comp,
	*tlb.Comp,
	*tlb.Comp,
) {
	pageTable := setupPageTable(*maxAddressFlag, s)

	mmuSpec := mmu.Definition.DefaultSpec
	mmuSpec.Log2PageSize = 12
	mmuSpec.MaxRequestsInFlight = 16
	mmuSpec.Latency = 10
	IoMMU := mmu.Definition.Builder().
		WithSimulation(s).
		WithSpec(mmuSpec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		WithPorts(mmu.Ports{
			Top:     newPort("IoMMU.Top"),
			Control: newPort("IoMMU.Control"),
		}).
		Build("IoMMU")

	L2TLBMapper := &mem.SinglePortMapper{
		Port: IoMMU.Ports.Top.AsRemote(),
	}

	l2TLBSpec := tlb.Definition.DefaultSpec
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = 12
	l2TLBSpec.NumReqPerCycle = 4
	L2TLB := tlb.Definition.Builder().
		WithSimulation(s).
		WithSpec(l2TLBSpec).
		WithResources(tlb.Resources{TranslationProviderMapper: L2TLBMapper}).
		WithPorts(tlb.Ports{
			Top:     newPort("L2TLB.Top"),
			Bottom:  newPort("L2TLB.Bottom"),
			Control: newPort("L2TLB.Control"),
		}).
		Build("L2TLB")

	TLBMapper := &mem.SinglePortMapper{
		Port: L2TLB.Ports.Top.AsRemote(),
	}

	tlbSpec := tlb.Definition.DefaultSpec
	tlbSpec.NumWays = 8
	tlbSpec.NumSets = 8
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 2
	TLB := tlb.Definition.Builder().
		WithSimulation(s).
		WithSpec(tlbSpec).
		WithResources(tlb.Resources{TranslationProviderMapper: TLBMapper}).
		WithPorts(tlb.Ports{
			Top:     newPort("TLB.Top"),
			Bottom:  newPort("TLB.Bottom"),
			Control: newPort("TLB.Control"),
		}).
		Build("TLB")

	return IoMMU, TLB, L2TLB
}

func setupPageTable(maxAddress uint64, s *sim.Simulation) vm.PageTable {
	pageTable := vm.MakePageTableBuilder().
		WithSimulation(s).
		WithLog2PageSize(12).
		Build("PageTable")

	ptBase := uint64(0x100000)
	pageSize := uint64(4096)
	numEntries := (maxAddress-1)/pageSize + 1

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

func setupConnection(
	s *sim.Simulation,
	agent *memaccessagent.MemAccessAgent,
	ROB *rob.Comp,
	AT *addresstranslator.Comp,
	TLB, L2TLB *tlb.Comp,
	IoMMU *mmu.Comp,
	L1Cache *writethroughcache.Comp,
	L2Cache *writeback.Comp,
	memCtrl *idealmemcontroller.Comp,
) {
	connect(s, "Conn1",
		agent.Ports.Mem,
		ROB.Ports.Top,
	)
	connect(s, "ConnROB",
		ROB.Ports.Bottom,
		AT.Ports.Top,
	)
	connect(s, "Conn2",
		AT.Ports.Translation,
		TLB.Ports.Top,
	)
	connect(s, "Conn3",
		TLB.Ports.Bottom,
		L2TLB.Ports.Top,
	)
	connect(s, "Conn4",
		L2TLB.Ports.Bottom,
		IoMMU.Ports.Top,
	)
	connect(s, "Conn5",
		AT.Ports.Bottom,
		L1Cache.Ports.Top,
	)
	connect(s, "Conn6",
		L1Cache.Ports.Bottom,
		L2Cache.Ports.Top,
	)
	connect(s, "Conn7",
		L2Cache.Ports.Bottom,
		memCtrl.Ports.Top,
	)
}

func main() {
	flag.Parse()

	seed := *seedFlag
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)
	rand.Seed(seed)

	s, engine, agent := setupTest(seed)
	agent.TickLater()

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	if len(agent.State.PendingWriteReq) > 0 || len(agent.State.PendingReadReq) > 0 {
		panic("Not all req returned")
	}

	if agent.State.WriteLeft > 0 || agent.State.ReadLeft > 0 {
		panic("more requests to send")
	}

	s.Terminate()
}
