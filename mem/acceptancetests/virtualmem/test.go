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

//nolint:funlen
func setupTest() (sim.Engine, *memaccessagent.MemAccessAgent) {
	s := simulation.MakeBuilder().Build()
	var engine sim.Engine
	if *parallelFlag {
		engine = sim.NewParallelEngine()
	} else {
		engine = sim.NewSerialEngine()
	}

	memCtrl := idealmemcontroller.MakeBuilder().
		WithEngine(engine).
		WithNewStorage(4 * mem.GB).
		WithLatency(100).
		Build("MemCtrl")
	s.RegisterComponent(memCtrl)

	pageTable := setupPageTable()

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

	IoMMU := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithLog2PageSize(12).
		WithMaxNumReqInFlight(16).
		WithPageWalkingLatency(10).
		WithPageTable(pageTable).
		Build("IoMMU")
	s.RegisterComponent(IoMMU)

	L2TLBmapper := &mem.SinglePortMapper{
		Port: IoMMU.GetPortByName("Top").AsRemote(),
	}

	L2TLB := tlb.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithNumWays(64).
		WithNumSets(64).
		WithPageSize(4096).
		WithNumReqPerCycle(4).
		//WithLowModule(IoMMU.GetPortByName("Top").AsRemote()).
		//WithAddressMapperType("single").
		WithAddressMapper(L2TLBmapper).
		Build("L2TLB")
	s.RegisterComponent(L2TLB)

	TLBmapper := &mem.SinglePortMapper{
		Port: L2TLB.GetPortByName("Top").AsRemote(),
	}

	TLB := tlb.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithNumWays(8).
		WithNumSets(8).
		WithPageSize(4096).
		WithNumReqPerCycle(2).
		//WithLowModule(L2TLB.GetPortByName("Top").AsRemote()).
		//WithAddressMapperType("single").
		WithAddressMapper(TLBmapper).
		Build("TLB")
	s.RegisterComponent(TLB)

	ATmapper := &mem.SinglePortMapper{
		Port: L1Cache.GetPortByName("Top").AsRemote(),
	}

	AT := addresstranslator.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithLog2PageSize(12).
		WithNumReqPerCycle(4).
		WithTranslationProvider(TLB.GetPortByName("Top").AsRemote()).
		//WithRemotePorts(L1Cache.GetPortByName("Top").AsRemote()).
		//WithAddressMapperType("single").
		WithAddressToPortMapper(ATmapper).
		Build("AT")
	s.RegisterComponent(AT)

	for portName := range AT.Ports() {
		fmt.Println("[AT] Registered Port:", portName)
	}

	agent = memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithMaxAddress(*maxAddressFlag).
		WithReadLeft(*numAccessFlag).
		WithWriteLeft(*numAccessFlag).
		WithLowModule(AT.GetPortByName("Top")).
		Build("MemAccessAgent")
	s.RegisterComponent(agent)

	setupConnection(engine, agent, AT, TLB, L2TLB, IoMMU, L1Cache, L2Cache, memCtrl)

	if *traceFileFlag != "" {
		traceFile, err := os.Create(*traceFileFlag)
		if err != nil {
			panic(err)
		}
		logger := log.New(traceFile, "", 0)
		tracer := trace.NewTracer(logger, engine)
		tracing.CollectTrace(memCtrl, tracer)
	}

	return engine, agent
}

func setupPageTable() vm.PageTable {
	// construct a page table
	pageTable := vm.NewPageTable(12) // 4096 = 2^12

	ptBase := uint64(0x100000) // physical starting Addr
	pageSize := uint64(4096)
	numEntries := 512

	for i := 0; i < numEntries; i++ {
		vAddr := uint64(i) * pageSize
		pAddr := ptBase + uint64(i)*pageSize
		page := vm.Page{
			PID:      1, // process ID
			VAddr:    vAddr,
			PAddr:    pAddr,
			PageSize: pageSize,
			Valid:    true,
		}
		pageTable.Insert(page)
	}

	return pageTable
}

func setupConnection(
	engine sim.Engine, agent,
	AT, TLB, L2TLB, IoMMU, L1Cache, L2Cache, memCtrl sim.Component,
) {
	Conn1 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn1")
	Conn1.PlugIn(agent.GetPortByName("Mem"))
	Conn1.PlugIn(AT.GetPortByName("Top"))

	Conn2 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn2")
	Conn2.PlugIn(AT.GetPortByName("Translation"))
	Conn2.PlugIn(TLB.GetPortByName("Top"))

	Conn3 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn3")
	Conn3.PlugIn(TLB.GetPortByName("Bottom"))
	Conn3.PlugIn(L2TLB.GetPortByName("Top"))

	Conn4 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn4")
	Conn4.PlugIn(L2TLB.GetPortByName("Bottom"))
	Conn4.PlugIn(IoMMU.GetPortByName("Top"))

	Conn5 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn5")
	Conn5.PlugIn(AT.GetPortByName("Bottom"))
	Conn5.PlugIn(L1Cache.GetPortByName("Top"))

	Conn6 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn6")
	Conn6.PlugIn(L1Cache.GetPortByName("Bottom"))
	Conn6.PlugIn(L2Cache.GetPortByName("Top"))

	Conn7 := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn7")
	Conn7.PlugIn(L2Cache.GetPortByName("Bottom"))
	Conn7.PlugIn(memCtrl.GetPortByName("Top"))
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
