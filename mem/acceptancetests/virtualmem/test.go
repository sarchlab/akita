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
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 10000, "Number of accesses")
var maxAddressFlag = flag.Uint64("max-address", 1*mem.GB, "Max memory address")

var traceFlag = flag.Bool("trace", false, "Collect trace")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

var agent *memaccessagent.MemAccessAgent

func setupTest() (*simulation.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
	simBuilder := simulation.MakeBuilder()

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()

	engine := s.GetEngine()

	l1Cache, l2Cache, memCtrl := buildMemoryHierarchy(s)
	ioMMU, tlb, l2TLB := buildTranslationHierarchy(s)

	atMemoryMapper := &mem.SinglePortMapper{
		Port: l1Cache.GetPortByName("Top").AsRemote(),
	}
	atTranslationMapper := &mem.SinglePortMapper{
		Port: tlb.GetPortByName("Top").AsRemote(),
	}

	atSpec := addresstranslator.DefaultSpec()
	atSpec.Log2PageSize = 12
	atSpec.NumReqPerCycle = 4
	at := addresstranslator.MakeBuilder().
		WithRegistrar(s).
		WithSpec(atSpec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper:         atMemoryMapper,
			TranslationProviderMapper: atTranslationMapper,
		}).
		Build("AT")

	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.WriteLeft = *numAccessFlag
	agent = memaccessagent.MakeBuilder().
		WithRegistrar(s).
		WithSpec(agentSpec).
		WithResources(memaccessagent.Resources{
			LowModule: at.GetPortByName("Top"),
		}).
		Build("MemAccessAgent")
	if monitor := s.GetMonitor(); monitor != nil {
		agent.CreateProgressBars(monitor.CreateProgressBar)
	}

	setupConnection(s, agent,
		at, tlb, l2TLB, ioMMU,
		l1Cache, l2Cache, memCtrl)

	return s, engine, agent
}

func buildMemoryHierarchy(s *simulation.Simulation) (
	*modeling.Component[writethroughcache.Spec, writethroughcache.State, writethroughcache.Resources],
	*modeling.Component[writeback.Spec, writeback.State, writeback.Resources],
	*idealmemcontroller.Comp,
) {
	memCtrlSpec := idealmemcontroller.DefaultSpec()
	memCtrlSpec.Capacity = 4 * mem.GB
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64
	memCtrl := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(memCtrlSpec).
		Build("MemCtrl")
	memCtrlTop := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(memCtrl).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Top")
	memCtrl.AssignPort("Top", memCtrlTop)
	memCtrlCtrl := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(memCtrl).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Control")
	memCtrl.AssignPort("Control", memCtrlCtrl)

	l2Spec := writeback.DefaultSpec()
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Spec.AddressMapperType = "single"
	L2Cache := writeback.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			RemotePorts: []messaging.RemotePort{
				memCtrl.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L2Cache")

	l1Spec := writethroughcache.DefaultSpec()
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2
	l1Spec.AddressMapperType = "single"
	L1Cache := writethroughcache.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l1Spec).
		WithResources(writethroughcache.Resources{
			RemotePorts: []messaging.RemotePort{
				L2Cache.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L1Cache")

	return L1Cache, L2Cache, memCtrl
}

func buildTranslationHierarchy(
	s *simulation.Simulation,
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
	IoMMU := mmu.MakeBuilder().
		WithRegistrar(s).
		WithSpec(mmuSpec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("IoMMU")

	L2TLBMapper := &mem.SinglePortMapper{
		Port: IoMMU.GetPortByName("Top").AsRemote(),
	}

	l2TLBSpec := tlb.DefaultSpec()
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = 12
	l2TLBSpec.NumReqPerCycle = 4
	L2TLB := tlb.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2TLBSpec).
		WithResources(tlb.Resources{TranslationProviderMapper: L2TLBMapper}).
		Build("L2TLB")

	TLBMapper := &mem.SinglePortMapper{
		Port: L2TLB.GetPortByName("Top").AsRemote(),
	}

	tlbSpec := tlb.DefaultSpec()
	tlbSpec.NumWays = 8
	tlbSpec.NumSets = 8
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 2
	TLB := tlb.MakeBuilder().
		WithRegistrar(s).
		WithSpec(tlbSpec).
		WithResources(tlb.Resources{TranslationProviderMapper: TLBMapper}).
		Build("TLB")

	return IoMMU, TLB, L2TLB
}

func setupPageTable(maxAddress uint64, s *simulation.Simulation) vm.PageTable {
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

func connect(s *simulation.Simulation, name string, p1, p2 messaging.Port) {
	conn := directconnection.MakeBuilder().WithRegistrar(s).Build(name)
	conn.PlugIn(p1)
	conn.PlugIn(p2)
}

func setupConnection(
	s *simulation.Simulation,
	agent *memaccessagent.MemAccessAgent,
	AT, TLB, L2TLB, IoMMU, L1Cache, L2Cache, memCtrl messaging.Component,
) {
	connect(s, "Conn1",
		agent.GetPortByName("Mem"),
		AT.GetPortByName("Top"),
	)
	connect(s, "Conn2",
		AT.GetPortByName("Translation"),
		TLB.GetPortByName("Top"),
	)
	connect(s, "Conn3",
		TLB.GetPortByName("Bottom"),
		L2TLB.GetPortByName("Top"),
	)
	connect(s, "Conn4",
		L2TLB.GetPortByName("Bottom"),
		IoMMU.GetPortByName("Top"),
	)
	connect(s, "Conn5",
		AT.GetPortByName("Bottom"),
		L1Cache.GetPortByName("Top"),
	)
	connect(s, "Conn6",
		L1Cache.GetPortByName("Bottom"),
		L2Cache.GetPortByName("Top"),
	)
	connect(s, "Conn7",
		L2Cache.GetPortByName("Bottom"),
		memCtrl.GetPortByName("Top"),
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

	s, engine, agent := setupTest()
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
