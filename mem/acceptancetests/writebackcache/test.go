package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/acceptancetests"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 100000,
	"Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")

// var traceFileFlag = flag.String("trace", "", "Trace file")
// var traceWithStdoutFlag = flag.Bool(
// "trace-stdout", false, "Trace with stdout")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

var sim simulation.Simulation
var engine timing.Engine
var agent *acceptancetests.MemAccessAgent

func main() {
	flag.Parse()

	initSeed()
	buildEnvironment()
	runSimulation()
	allMsgsMustBeSent()
}

func initSeed() {
	var seed int64
	if *seedFlag == 0 {
		seed = time.Now().UnixNano()
	} else {
		seed = *seedFlag
	}

	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)

	rand.Seed(seed)
}

func buildEnvironment() {
	sim = simulation.NewSimulation()

	if *parallelFlag {
		engine = timing.NewParallelEngine()
	} else {
		engine = timing.NewSerialEngine()
	}

	sim.RegisterEngine(engine)
	//engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("Conn")

	agent = acceptancetests.NewMemAccessAgent(sim)
	agent.MaxAddress = *maxAddressFlag
	agent.WriteLeft = *numAccessFlag
	agent.ReadLeft = *numAccessFlag

	addressToPortMapper := new(mem.SinglePortMapper)
	builder := cache.MakeBuilder().
		WithSimulation(sim).
		WithFreq(1 * timing.GHz).
		WithCycleLatency(4).
		WithAddressToDstTable(addressToPortMapper).
		WithLog2CacheLineSize(6).
		WithWayAssociativity(4).
		WithMSHRCapacity(4).
		WithNumReqPerCycle(16).
		WithWriteStrategy("writeback")

	writeBackCache := builder.Build("Cache")

	dram := idealmemcontroller.MakeBuilder().
		WithSimulation(sim).
		WithNewStorage(4 * mem.GB).
		Build("DRAM")
	addressToPortMapper.Port = dram.GetPortByName("Top").AsRemote()

	agent.LowModule = writeBackCache.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(writeBackCache.GetPortByName("Bottom"))
	conn.PlugIn(writeBackCache.GetPortByName("Top"))
	conn.PlugIn(dram.GetPortByName("Top"))

	agent.TickLater()
}

func runSimulation() {
	err := engine.Run()
	if err != nil {
		panic(err)
	}
}

func allMsgsMustBeSent() {
	if len(agent.PendingWriteReq) > 0 || len(agent.PendingReadReq) > 0 {
		panic("Not all req returned")
	}

	if agent.WriteLeft > 0 || agent.ReadLeft > 0 {
		panic("more requests to send")
	}
}
