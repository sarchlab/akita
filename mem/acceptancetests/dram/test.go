package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access",
	100000, "Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

func setupTest() (*simulation.Simulation, sim.Engine, *memaccessagent.MemAccessAgent) {
	simBuilder := simulation.MakeBuilder()

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}

	s := simBuilder.Build()
	engine := s.GetEngine()

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
	s.RegisterComponent(agent)

	memCtrl := dram.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithTopPort(sim.NewPort(nil, 1024, 1024, "Mem.TopPort")).
		Build("Mem")
	s.RegisterComponent(memCtrl)

	agent.LowModule = memCtrl.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(memCtrl.GetPortByName("Top"))

	return s, engine, agent
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

	if len(agent.GetState().PendingWriteReq) > 0 || len(agent.GetState().PendingReadReq) > 0 {
		panic("Not all req returned")
	}

	if agent.GetState().WriteLeft > 0 || agent.GetState().ReadLeft > 0 {
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
