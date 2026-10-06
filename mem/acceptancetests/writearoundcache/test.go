package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim/messaging"

	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
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
	monitor := monitoring.NewMonitor()
	simBuilder := sim.MakeBuilder().WithMonitor(monitor)

	if *parallelFlag {
		simBuilder = simBuilder.WithParallelEngine()
	}
	if *traceFlag {
		simBuilder = simBuilder.WithVisTracingOnStart()
	}

	s := simBuilder.Build()
	engine := s.Engine()

	conn := directconnection.MakeBuilder().
		WithSimulation(s).
		Build("Conn")

	// The agent sends to the cache's Top port, so the cache's ports are
	// created before the agent is built.
	cachePorts := writethroughcache.Ports{
		Top:     messaging.NewPort("Cache.Top", 16, 16),
		Bottom:  messaging.NewPort("Cache.Bottom", 16, 16),
		Control: messaging.NewPort("Cache.Control", 16, 16),
	}

	agentSpec := memaccessagent.Definition.DefaultSpec
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.RandSeed = seed
	agent := memaccessagent.Definition.Builder().
		WithSimulation(s).
		WithSpec(agentSpec).
		WithResources(memaccessagent.Resources{LowModule: cachePorts.Top}).
		WithPorts(memaccessagent.Ports{
			Mem: messaging.NewPort("MemAccessAgent.Mem", 16, 16),
		}).
		Build("MemAccessAgent")
	memaccessagent.SetProgressTrackers(agent,
		monitor.CreateProgressBar(agent.Name()+".Writes", uint64(agent.State.WriteLeft)),
		monitor.CreateProgressBar(agent.Name()+".Reads", uint64(agent.State.ReadLeft)),
	)

	dram := idealmemcontroller.Definition.Builder().
		WithSimulation(s).
		WithSpec(idealmemcontroller.Definition.DefaultSpec).
		WithResources(idealmemcontroller.Resources{
			Storage: mem.MakeStorageBuilder().
				WithCapacity(4 * mem.GB).
				WithSimulation(s).
				Build("DRAM.Storage"),
		}).
		WithPorts(idealmemcontroller.Ports{
			Top:     messaging.NewPort("DRAM.Top", 16, 16),
			Control: messaging.NewPort("DRAM.Control", 16, 16),
		}).
		Build("DRAM")

	addressToPortMapper := new(mem.SinglePortMapper)
	addressToPortMapper.Port = dram.Ports.Top.AsRemote()

	cacheSpec := writethroughcache.Definition.DefaultSpec
	cacheSpec.WritePolicyType = "write-around"
	cacheSpec.Log2BlockSize = 6
	cacheSpec.NumMSHREntry = 4
	cacheSpec.WayAssociativity = 8
	cacheSpec.TotalByteSize = 4 * mem.KB
	cacheSpec.NumBanks = 1
	cacheSpec.BankLatency = 20
	writeAroundCache := writethroughcache.Definition.Builder().
		WithSimulation(s).
		WithSpec(cacheSpec).
		WithResources(writethroughcache.Resources{
			Storage: mem.MakeStorageBuilder().
				WithCapacity(cacheSpec.TotalByteSize).
				WithSimulation(s).
				Build("Cache.Storage"),
			AddressMapper: addressToPortMapper,
		}).
		WithPorts(cachePorts).
		Build("Cache")

	conn.PlugIn(agent.Ports.Mem)
	conn.PlugIn(writeAroundCache.Ports.Bottom)
	conn.PlugIn(writeAroundCache.Ports.Top)
	conn.PlugIn(dram.Ports.Top)

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
