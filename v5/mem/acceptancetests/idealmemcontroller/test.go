package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/trace"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
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

	engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	agent := memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithMaxAddress(*maxAddressFlag).
		WithWriteLeft(*numAccessFlag).
		WithReadLeft(*numAccessFlag).
		WithMemPort(sim.NewPort(nil, 1, 1, "MemAccessAgent.Mem")).
		Build("MemAccessAgent")

	dram := idealmemcontroller.MakeBuilder().
		WithEngine(engine).
		WithNewStorage(4 * mem.GB).
		WithSpec(idealmemcontroller.Spec{Width: 1, Latency: 100, CacheLineSize: 64}).
		WithTopPort(sim.NewPort(nil, 16, 16, "DRAM.TopPort")).
		WithCtrlPort(sim.NewPort(nil, 16, 16, "DRAM.CtrlPort")).
		Build("DRAM")
	agent.LowModule = dram.GetPortByName("Top")

	if *traceFileFlag != "" {
		recorder := datarecording.NewDataRecorder(*traceFileFlag)
		tracer := trace.NewDBTracer(recorder, engine)
		tracing.CollectTrace(dram, tracer)
	}

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(dram.GetPortByName("Top"))

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
