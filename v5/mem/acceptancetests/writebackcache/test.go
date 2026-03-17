package main

import (
	"fmt"
	"math/rand"

	"time"

	"flag"

	"os"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/trace"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 100000,
	"Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var traceFileFlag = flag.String("trace", "", "Trace file")
var traceWithStdoutFlag = flag.Bool("trace-stdout", false, "Trace with stdout")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

var engine sim.Engine
var agent *memaccessagent.MemAccessAgent

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
	if *parallelFlag {
		engine = sim.NewParallelEngine()
	} else {
		engine = sim.NewSerialEngine()
	}
	//engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	agent = memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithMaxAddress(*maxAddressFlag).
		WithWriteLeft(*numAccessFlag).
		WithReadLeft(*numAccessFlag).
		WithMemPort(sim.NewPort(nil, 1, 1, "MemAccessAgent.Mem")).
		Build("MemAccessAgent")

	dram := idealmemcontroller.MakeBuilder().
		WithEngine(engine).
		WithNewStorage(4 * mem.GB).
		WithTopPort(sim.NewPort(nil, 16, 16, "DRAM.TopPort")).
		WithCtrlPort(sim.NewPort(nil, 16, 16, "DRAM.CtrlPort")).
		Build("DRAM")

	addressToPortMapper := new(mem.SinglePortMapper)
	addressToPortMapper.Port = dram.GetPortByName("Top").AsRemote()

	builder := writeback.MakeBuilder().
		WithEngine(engine).
		WithAddressToPortMapper(addressToPortMapper).
		WithByteSize(16 * mem.KB).
		WithLog2BlockSize(6).
		WithWayAssociativity(4).
		WithNumMSHREntry(4).
		WithNumReqPerCycle(16).
		WithTopPort(sim.NewPort(nil, 32, 32, "Cache.ToTop")).
		WithBottomPort(sim.NewPort(nil, 32, 32, "Cache.BottomPort")).
		WithControlPort(sim.NewPort(nil, 32, 32, "Cache.ControlPort"))
	writeBackCache := builder.Build("Cache")

	setupTracing(writeBackCache)

	agent.LowModule = writeBackCache.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(writeBackCache.GetPortByName("Bottom"))
	conn.PlugIn(writeBackCache.GetPortByName("Top"))
	conn.PlugIn(dram.GetPortByName("Top"))

	agent.TickLater()
}

func setupTracing(comp tracing.NamedHookable) {
	if *traceWithStdoutFlag {
		tmpFile, err := os.CreateTemp("", "trace-stdout-*.db")
		if err != nil {
			panic(err)
		}
		tmpFile.Close()
		recorder := datarecording.NewDataRecorder(tmpFile.Name())
		tracer := trace.NewDBTracer(recorder, engine)
		tracing.CollectTrace(comp, tracer)
	} else if *traceFileFlag != "" {
		recorder := datarecording.NewDataRecorder(*traceFileFlag)
		tracer := trace.NewDBTracer(recorder, engine)
		tracing.CollectTrace(comp, tracer)
	}
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
