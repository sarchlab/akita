package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/rob"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 10000, "Number of accesses")
var maxAddressFlag = flag.Uint64("max-address", 1*mem.GB, "Max memory address")

var traceFlag = flag.Bool("trace", false, "Collect trace")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

var agent *memaccessagent.Comp

//nolint:funlen // wires the whole simulation in one place
func setupTest(
	seed int64,
) (*sim.Simulation, timing.Engine, *memaccessagent.Comp) {
	monitor := monitoring.MakeBuilder().Build()
	simBuilder := sim.MakeBuilder().WithMonitor(monitor)

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

	atSpec := addresstranslator.DefaultSpec()
	atSpec.Log2PageSize = 12
	atSpec.NumReqPerCycle = 4

	atSpec.Bottom.Destinations = []messaging.RemotePort{l1Cache.Ports.Top.AsRemote()}

	atSpec.Translation.Destinations = []messaging.RemotePort{tlb.Ports.Top.AsRemote()}
	at := modeling.MakeBuilder[*addresstranslator.Comp](atSpec).
		WithSimulation(s).
		Build("AT")

	at.BindPort("Top", newPort("AT.Top"))
	at.BindPort("Bottom", newPort("AT.Bottom"))
	at.BindPort("Translation", newPort("AT.Translation"))
	at.BindPort("Control", newPort("AT.Control"))

	// Insert a reorder buffer between the agent and the address translator so
	// the trace exercises the ROB instrumentation end-to-end.
	robComp := buildROB(s, at.Ports.Top.AsRemote())

	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.RandSeed = seed
	agentSpec.LowModule = robComp.Ports.Top.AsRemote()
	agent = modeling.MakeBuilder[*memaccessagent.Comp](agentSpec).
		WithSimulation(s).
		Build("MemAccessAgent")

	agent.BindPort("Mem", newPort("MemAccessAgent.Mem"))

	setupConnection(s, agent, robComp,
		at, tlb, l2TLB, ioMMU,
		l1Cache, l2Cache, memCtrl)

	if err := s.Initialize(); err != nil {
		panic(err)
	}
	memaccessagent.SetProgressTrackers(agent,
		monitor.CreateProgressBar(agent.Name()+".Writes", uint64(agent.State.WriteLeft)),
		monitor.CreateProgressBar(agent.Name()+".Reads", uint64(agent.State.ReadLeft)),
	)

	return s, engine, agent
}

// buildROB builds a reorder buffer that forwards every access to bottomUnit
// (the address translator's Top port) and reorders the responses back.
func buildROB(s *sim.Simulation, bottomUnit messaging.RemotePort) *rob.Comp {
	robSpec := rob.DefaultSpec()
	robSpec.NumReqPerCycle = 4
	robSpec.Bottom = bottomUnit

	builtComponent := modeling.MakeBuilder[*rob.Comp](robSpec).
		WithSimulation(s).
		Build("ROB")

	builtComponent.BindPort("Top", newPort("ROB.Top"))
	builtComponent.BindPort("Bottom", newPort("ROB.Bottom"))
	builtComponent.BindPort("Control", newPort("ROB.Control"))
	return builtComponent
}

//nolint:funlen // wires the whole hierarchy in one place
func buildMemoryHierarchy(s *sim.Simulation) (
	*writethrough.Comp,
	*writeback.Comp,
	*idealmemcontroller.Comp,
) {
	memCtrlSpec := idealmemcontroller.DefaultSpec()
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrl := modeling.MakeBuilder[*idealmemcontroller.Comp](memCtrlSpec).
		WithSimulation(s).
		WithResources(idealmemcontroller.Resources{
			Storage: newStorage(s, 4*mem.GB, "MemCtrl.Storage"),
		}).
		Build("MemCtrl")

	memCtrl.BindPort("Top", newPort("MemCtrl.Top"))
	memCtrl.BindPort("Control", newPort("MemCtrl.Control"))

	l2Spec := writeback.DefaultSpec()
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2

	l2Spec.Bottom.Destinations = []messaging.RemotePort{
		memCtrl.Ports.Top.AsRemote(),
	}
	L2Cache := modeling.MakeBuilder[*writeback.Comp](l2Spec).
		WithSimulation(s).
		WithResources(writeback.Resources{Storage: newStorage(s, l2Spec.TotalByteSize, "L2Cache.Storage")}).
		Build("L2Cache")

	L2Cache.BindPort("Top", newPort("L2Cache.Top"))
	L2Cache.BindPort("Bottom", newPort("L2Cache.Bottom"))
	L2Cache.BindPort("Control", newPort("L2Cache.Control"))

	l1Spec := writethrough.DefaultSpec()
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2

	l1Spec.Bottom.Destinations = []messaging.RemotePort{
		L2Cache.Ports.Top.AsRemote(),
	}
	L1Cache := modeling.MakeBuilder[*writethrough.Comp](l1Spec).
		WithSimulation(s).
		WithResources(writethrough.Resources{Storage: newStorage(s, l1Spec.TotalByteSize, "L1Cache.Storage")}).
		Build("L1Cache")

	L1Cache.BindPort("Top", newPort("L1Cache.Top"))
	L1Cache.BindPort("Bottom", newPort("L1Cache.Bottom"))
	L1Cache.BindPort("Control", newPort("L1Cache.Control"))

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

	mmuSpec := mmu.DefaultSpec()
	mmuSpec.Log2PageSize = 12
	mmuSpec.MaxRequestsInFlight = 16
	mmuSpec.Latency = 10
	IoMMU := modeling.MakeBuilder[*mmu.Comp](mmuSpec).
		WithSimulation(s).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("IoMMU")

	IoMMU.BindPort("Top", newPort("IoMMU.Top"))
	IoMMU.BindPort("Control", newPort("IoMMU.Control"))

	L2TLBMapper := IoMMU.Ports.Top.AsRemote()

	l2TLBSpec := tlb.DefaultSpec()
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = 12
	l2TLBSpec.NumReqPerCycle = 4

	l2TLBSpec.Bottom.Destinations = []messaging.RemotePort{L2TLBMapper}
	L2TLB := modeling.MakeBuilder[*tlb.Comp](l2TLBSpec).
		WithSimulation(s).
		Build("L2TLB")

	L2TLB.BindPort("Top", newPort("L2TLB.Top"))
	L2TLB.BindPort("Bottom", newPort("L2TLB.Bottom"))
	L2TLB.BindPort("Control", newPort("L2TLB.Control"))

	tlbSpec := tlb.DefaultSpec()
	tlbSpec.NumWays = 8
	tlbSpec.NumSets = 8
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 2

	tlbSpec.Bottom.Destinations = []messaging.RemotePort{L2TLB.Ports.Top.AsRemote()}
	TLB := modeling.MakeBuilder[*tlb.Comp](tlbSpec).
		WithSimulation(s).
		Build("TLB")

	TLB.BindPort("Top", newPort("TLB.Top"))
	TLB.BindPort("Bottom", newPort("TLB.Bottom"))
	TLB.BindPort("Control", newPort("TLB.Control"))

	return IoMMU, TLB, L2TLB
}

func setupPageTable(maxAddress uint64, s *sim.Simulation) *vm.PageTable {
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
// its ports during wiring. BindPort names, binds, and registers it.
func newPort(fullName string) messaging.Port {
	return twowaybuffered.NewPort(16, 16)
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
	conn := direct.NewConnection(name, s, timing.GHz)
	conn.BindPort(p1)
	conn.BindPort(p2)
}

func setupConnection(
	s *sim.Simulation,
	agent *memaccessagent.Comp,
	ROB *rob.Comp,
	AT *addresstranslator.Comp,
	TLB, L2TLB *tlb.Comp,
	IoMMU *mmu.Comp,
	L1Cache *writethrough.Comp,
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

	if err := s.Terminate(); err != nil {
		panic(err)
	}
}
