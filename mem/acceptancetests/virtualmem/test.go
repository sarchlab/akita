package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/sarchlab/akita/v4/mem/cache/writeback"
	"github.com/sarchlab/akita/v4/mem/cache/writethrough"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/trace"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/tracing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"

	"github.com/sarchlab/akita/v4/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v4/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v4/mem/vm/mmu"
	"github.com/sarchlab/akita/v4/mem/vm/tlb"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 10000, "Number of accesses")
var maxAddressFlag = flag.Uint64("max-address", 1*mem.GB, "Max memory address")

var traceFileFlag = flag.String("trace", "", "Trace file")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

var agent *memaccessagent.MemAccessAgent

func setupTest() (sim.Engine, *memaccessagent.MemAccessAgent) {
	simBuilder := simulation.MakeBuilder()

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}

	s := simBuilder.Build()

	engine := s.GetEngine()

	l1Cache, l2Cache, memCtrl := buildMemoryHierarchy(engine, s)
	ioMMU, tlb, l2TLB := buildTranslationHierarchy(engine, s)

	atMemoryMapper := &mem.SinglePortMapper{
		Port: l1Cache.GetPortByName("Top").AsRemote(),
	}
	atTranslationMapper := &mem.SinglePortMapper{
		Port: tlb.GetPortByName("Top").AsRemote(),
	}

	at := addresstranslator.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithLog2PageSize(12).
		WithNumReqPerCycle(4).
		WithMemoryProviderMapper(atMemoryMapper).
		WithTranslationProviderMapper(atTranslationMapper).
		Build("AT")
	s.RegisterComponent(at)

	agent = memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithMaxAddress(*maxAddressFlag).
		WithReadLeft(*numAccessFlag).
		WithWriteLeft(*numAccessFlag).
		WithLowModule(at.GetPortByName("Top")).
		Build("MemAccessAgent")
	s.RegisterComponent(agent)

	setupConnection(engine, agent,
		at, tlb, l2TLB, ioMMU,
		l1Cache, l2Cache, memCtrl)
	setupTracing(engine, memCtrl)

	return engine, agent
}

func buildMemoryHierarchy(engine sim.Engine, s *simulation.Simulation) (
	*writethrough.Comp, *writeback.Comp, *idealmemcontroller.Comp,
) {
	memCtrl := idealmemcontroller.MakeBuilder().
		WithEngine(engine).
		WithNewStorage(4 * mem.GB).
		WithLatency(100).
		Build("MemCtrl")
	s.RegisterComponent(memCtrl)

	L2Cache := writeback.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWayAssociativity(4).
		WithNumReqPerCycle(2).
		WithAddressMapperType("single").
		WithRemotePorts(memCtrl.GetPortByName("Top").AsRemote()).
		Build("L2Cache")
	s.RegisterComponent(L2Cache)

	L1Cache := writethrough.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWayAssociativity(2).
		WithAddressMapperType("single").
		WithRemotePorts(L2Cache.GetPortByName("Top").AsRemote()).
		Build("L1Cache")
	s.RegisterComponent(L1Cache)

	return L1Cache, L2Cache, memCtrl
}

func buildTranslationHierarchy(engine sim.Engine, s *simulation.Simulation) (
	*mmu.Comp, *tlb.Comp, *tlb.Comp,
) {
	pageTable := setupPageTable(*maxAddressFlag)

	IoMMU := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithLog2PageSize(12).
		WithMaxNumReqInFlight(16).
		WithPageWalkingLatency(10).
		WithPageTable(pageTable).
		Build("IoMMU")
	s.RegisterComponent(IoMMU)

	L2TLBMapper := &mem.SinglePortMapper{
		Port: IoMMU.GetPortByName("Top").AsRemote(),
	}

	L2TLB := tlb.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithNumWays(64).
		WithNumSets(64).
		WithLog2PageSize(12).
		WithNumReqPerCycle(4).
		WithTranslationProviderMapper(L2TLBMapper).
		Build("L2TLB")
	s.RegisterComponent(L2TLB)

	TLBMapper := &mem.SinglePortMapper{
		Port: L2TLB.GetPortByName("Top").AsRemote(),
	}

	TLB := tlb.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithNumWays(8).
		WithNumSets(8).
		WithLog2PageSize(12).
		WithNumReqPerCycle(2).
		WithTranslationProviderMapper(TLBMapper).
		Build("TLB")
	s.RegisterComponent(TLB)

	return IoMMU, TLB, L2TLB
}

func setupPageTable(maxAddress uint64) vm.PageTable {
	pageTable := vm.NewPageTable(12)

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

func connect(engine sim.Engine, name string, p1, p2 sim.Port) {
	conn := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build(name)
	conn.PlugIn(p1)
	conn.PlugIn(p2)
}

func setupConnection(
	engine sim.Engine,
	agent *memaccessagent.MemAccessAgent,
	AT, TLB, L2TLB, IoMMU, L1Cache, L2Cache, memCtrl sim.Component,
) {
	connect(engine, "Conn1",
		agent.GetPortByName("Mem"),
		AT.GetPortByName("Top"),
	)
	connect(engine, "Conn2",
		AT.GetPortByName("Translation"),
		TLB.GetPortByName("Top"),
	)
	connect(engine, "Conn3",
		TLB.GetPortByName("Bottom"),
		L2TLB.GetPortByName("Top"),
	)
	connect(engine, "Conn4",
		L2TLB.GetPortByName("Bottom"),
		IoMMU.GetPortByName("Top"),
	)
	connect(engine, "Conn5",
		AT.GetPortByName("Bottom"),
		L1Cache.GetPortByName("Top"),
	)
	connect(engine, "Conn6",
		L1Cache.GetPortByName("Bottom"),
		L2Cache.GetPortByName("Top"),
	)
	connect(engine, "Conn7",
		L2Cache.GetPortByName("Bottom"),
		memCtrl.GetPortByName("Top"),
	)
}

func setupTracing(engine sim.Engine, memCtrl *idealmemcontroller.Comp) {
	if *traceFileFlag == "" {
		return
	}

	traceFile, err := os.Create(*traceFileFlag)
	if err != nil {
		panic(err)
	}

	logger := log.New(traceFile, "", 0)
	tracer := trace.NewTracer(logger, engine)
	tracing.CollectTrace(memCtrl, tracer)
}

func main() {
	flag.Parse()

	seed := *seedFlag
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)
	rand.Seed(seed)

	engine, agent := setupTest()
	agent.TickLater()

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	if len(agent.PendingWriteReq) > 0 || len(agent.PendingReadReq) > 0 {
		panic("Not all req returned")
	}

	if agent.WriteLeft > 0 || agent.ReadLeft > 0 {
		panic("more requests to send")
	}

	entries, _ := os.ReadDir(".")
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "akita_sim_") && strings.HasSuffix(entry.Name(), ".sqlite3") {
			os.Remove(entry.Name())
		}
	}
}
