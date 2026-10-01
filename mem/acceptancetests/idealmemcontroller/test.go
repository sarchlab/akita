package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/directconnection"

	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access",
	100000, "Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")
var traceFlag = flag.Bool("trace", false, "Collect trace")

func setupTest(seed int64) (*simulation.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
	simBuilder := simulation.MakeBuilder()

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()
	engine := s.GetEngine()

	conn := directconnection.MakeBuilder().
		WithSimulation(s).
		Build("Conn")

	// The agent sends to the DRAM's Top port, so the DRAM's ports are created
	// before the agent is built.
	dramPorts := idealmemcontroller.Ports{
		Top:     messaging.NewPort("DRAM.Top", 16, 16),
		Control: messaging.NewPort("DRAM.Control", 16, 16),
	}

	agentSpec := memaccessagent.Definition.DefaultSpec
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.RandSeed = seed
	agent := memaccessagent.Definition.Builder().
		WithSimulation(s).
		WithSpec(agentSpec).
		WithResources(memaccessagent.Resources{LowModule: dramPorts.Top}).
		WithPorts(memaccessagent.Ports{
			Mem: messaging.NewPort("MemAccessAgent.Mem", 16, 16),
		}).
		Build("MemAccessAgent")
	if monitor := s.GetMonitor(); monitor != nil {
		memaccessagent.CreateProgressBars(agent, monitor.CreateProgressBar)
	}

	dramSpec := idealmemcontroller.Definition.DefaultSpec
	dramSpec.Width = 1
	dramSpec.Latency = 100
	dramSpec.CacheLineSize = 64
	dram := idealmemcontroller.Definition.Builder().
		WithSimulation(s).
		WithSpec(dramSpec).
		WithResources(idealmemcontroller.Resources{
			Storage: mem.MakeStorageBuilder().
				WithCapacity(4 * mem.GB).
				WithSimulation(s).
				Build("DRAM.Storage"),
		}).
		WithPorts(dramPorts).
		Build("DRAM")

	conn.PlugIn(agent.Ports.Mem)
	conn.PlugIn(dram.Ports.Top)

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

	s.Terminate()
}
