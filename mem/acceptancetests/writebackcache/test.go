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
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 100000,
	"Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")
var traceFlag = flag.Bool("trace", false, "Collect trace")

//nolint:funlen // wires the whole simulation in one place
func buildEnvironment(
	seed int64,
) (*sim.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
	monitor := monitoring.MakeBuilder().Build()
	simBuilder := sim.MakeBuilder().WithMonitor(monitor)

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()
	engine := s.Engine()

	conn := direct.NewConnection("Conn", s, timing.GHz)

	// The agent sends to the cache's Top port, so the cache's ports are
	// created before the agent is built.
	cachePorts := writeback.Ports{
		Top:     twowaybuffered.NewPort(16, 16),
		Bottom:  twowaybuffered.NewPort(16, 16),
		Control: twowaybuffered.NewPort(16, 16),
	}

	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.RandSeed = seed
	agentSpec.LowModule = "Cache.Top"
	agent := modeling.MakeBuilder[*memaccessagent.Comp](agentSpec).
		WithSimulation(s).
		Build("MemAccessAgent")

	agent.BindPort("Mem", twowaybuffered.NewPort(16, 16))

	dram := buildDRAM(s)

	cacheSpec := writeback.DefaultSpec()
	cacheSpec.TotalByteSize = 16 * mem.KB
	cacheSpec.Log2BlockSize = 6
	cacheSpec.WayAssociativity = 4
	cacheSpec.NumMSHREntry = 4
	cacheSpec.NumReqPerCycle = 16

	cacheSpec.Bottom.Destinations = []messaging.RemotePort{dram.Ports.Top.AsRemote()}
	writeBackCache := modeling.MakeBuilder[*writeback.Comp](cacheSpec).
		WithSimulation(s).
		WithResources(writeback.Resources{Storage: mem.MakeStorageBuilder().
			WithCapacity(cacheSpec.TotalByteSize).
			WithSimulation(s).
			Build("Cache.Storage")}).
		Build("Cache")
	writeBackCachePorts := cachePorts
	writeBackCache.BindPort("Top", writeBackCachePorts.Top)
	writeBackCache.BindPort("Bottom", writeBackCachePorts.Bottom)
	writeBackCache.BindPort("Control", writeBackCachePorts.Control)

	conn.BindPort(agent.Ports.Mem)
	conn.BindPort(writeBackCache.Ports.Bottom)
	conn.BindPort(writeBackCache.Ports.Top)
	conn.BindPort(dram.Ports.Top)
	if err := s.Initialize(); err != nil {
		panic(err)
	}
	memaccessagent.SetProgressTrackers(agent,
		monitor.CreateProgressBar(agent.Name()+".Writes", uint64(agent.State.WriteLeft)),
		monitor.CreateProgressBar(agent.Name()+".Reads", uint64(agent.State.ReadLeft)),
	)

	return s, engine, agent
}

// buildDRAM builds and registers the backing ideal memory controller.
func buildDRAM(s *sim.Simulation) *idealmemcontroller.Comp {
	dram := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
		WithSimulation(s).
		WithResources(idealmemcontroller.Resources{
			Storage: mem.MakeStorageBuilder().
				WithCapacity(4 * mem.GB).
				WithSimulation(s).
				Build("DRAM.Storage"),
		}).
		Build("DRAM")

	dram.BindPort("Top", twowaybuffered.NewPort(16, 16))
	dram.BindPort("Control", twowaybuffered.NewPort(16, 16))

	return dram
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

	s, engine, agent := buildEnvironment(seed)
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
