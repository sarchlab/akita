package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/noc/directconnection"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 100000,
	"Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

func buildEnvironment() (*simulation.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
	simBuilder := simulation.MakeBuilder()

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}

	s := simBuilder.Build()
	engine := s.GetEngine()

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("Conn")

	agent := memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithMaxAddress(*maxAddressFlag).
		WithWriteLeft(*numAccessFlag).
		WithReadLeft(*numAccessFlag).
		WithMemPort(messaging.NewPort(nil, 1, 1, "MemAccessAgent.Mem")).
		Build("MemAccessAgent")
	s.RegisterComponent(agent)

	dram := idealmemcontroller.MakeBuilder().
		WithEngine(engine).
		WithNewStorage(4 * mem.GB).
		WithTopPort(messaging.NewPort(nil, 16, 16, "DRAM.TopPort")).
		WithCtrlPort(messaging.NewPort(nil, 16, 16, "DRAM.CtrlPort")).
		Build("DRAM")
	s.RegisterComponent(dram)

	addressToPortMapper := new(mem.SinglePortMapper)
	addressToPortMapper.Port = dram.GetPortByName("Top").AsRemote()

	writeBackCache := writeback.MakeBuilder().
		WithEngine(engine).
		WithAddressToPortMapper(addressToPortMapper).
		WithByteSize(16 * mem.KB).
		WithLog2BlockSize(6).
		WithWayAssociativity(4).
		WithNumMSHREntry(4).
		WithNumReqPerCycle(16).
		WithTopPort(messaging.NewPort(nil, 32, 32, "Cache.ToTop")).
		WithBottomPort(messaging.NewPort(nil, 32, 32, "Cache.BottomPort")).
		WithControlPort(messaging.NewPort(nil, 32, 32, "Cache.ControlPort")).
		Build("Cache")
	s.RegisterComponent(writeBackCache)

	agent.LowModule = writeBackCache.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(writeBackCache.GetPortByName("Bottom"))
	conn.PlugIn(writeBackCache.GetPortByName("Top"))
	conn.PlugIn(dram.GetPortByName("Top"))

	return s, engine, agent
}

func main() {
	flag.Parse()

	var seed int64
	if *seedFlag == 0 {
		seed = time.Now().UnixNano()
	} else {
		seed = *seedFlag
	}

	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)
	rand.Seed(seed)

	s, engine, agent := buildEnvironment()
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

	entries, _ := os.ReadDir(".")
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "akita_sim_") && strings.HasSuffix(entry.Name(), ".sqlite3") {
			os.Remove(entry.Name())
		}
	}
}
