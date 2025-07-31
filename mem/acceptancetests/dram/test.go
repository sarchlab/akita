package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v4/mem/dram"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"

	"os"
	"time"

	"log"

	"github.com/sarchlab/akita/v4/mem/trace"
	"github.com/sarchlab/akita/v4/tracing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access",
	100000, "Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var traceFileFlag = flag.String("trace", "", "Trace file")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

func setupTest() (sim.Engine, *memaccessagent.MemAccessAgent) {
	var engine sim.Engine
	if *parallelFlag {
		engine = sim.NewParallelEngine()
	} else {
		engine = sim.NewSerialEngine()
	}

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	agent := memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithMaxAddress(*maxAddressFlag).
		WithWriteLeft(*numAccessFlag).
		WithReadLeft(*numAccessFlag).
		Build("MemAccessAgent")

	memCtrl := dram.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Mem")

	agent.LowModule = memCtrl.GetPortByName("Top")

	if *traceFileFlag != "" {
		traceFile, _ := os.Create(*traceFileFlag)
		logger := log.New(traceFile, "", 0)
		tracer := trace.NewTracer(logger, engine)
		tracing.CollectTrace(memCtrl, tracer)
	}

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(memCtrl.GetPortByName("Top"))

	return engine, agent
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
}
