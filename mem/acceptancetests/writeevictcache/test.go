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

	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 100000,
	"Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")
var traceFlag = flag.Bool("trace", false, "Collect trace")

func buildEnvironment() (*simulation.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) { //nolint:funlen
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
		WithRegistrar(s).
		Build("Conn")

	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.ReadLeft = *numAccessFlag
	agent := memaccessagent.MakeBuilder().
		WithRegistrar(s).
		WithSpec(agentSpec).
		Build("MemAccessAgent")
	if monitor := s.GetMonitor(); monitor != nil {
		agent.CreateProgressBars(monitor.CreateProgressBar)
	}

	dramSpec := idealmemcontroller.DefaultSpec()
	dramSpec.Capacity = 4 * mem.GB
	dram := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(dramSpec).
		Build("DRAM")

	addressToPortMapper := new(mem.SinglePortMapper)
	addressToPortMapper.Port = dram.GetPortByName("Top").AsRemote()

	cacheSpec := writethroughcache.DefaultSpec()
	cacheSpec.WritePolicyType = "write-evict"
	cacheSpec.Log2BlockSize = 6
	cacheSpec.NumMSHREntry = 4
	cacheSpec.WayAssociativity = 8
	cacheSpec.TotalByteSize = 4 * mem.KB
	cacheSpec.NumBanks = 1
	cacheSpec.BankLatency = 20
	writeEvictCache := writethroughcache.MakeBuilder().
		WithRegistrar(s).
		WithSpec(cacheSpec).
		WithResources(writethroughcache.Resources{
			AddressMapper: addressToPortMapper,
		}).
		Build("Cache")

	agent.LowModule = writeEvictCache.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(writeEvictCache.GetPortByName("Bottom"))
	conn.PlugIn(writeEvictCache.GetPortByName("Top"))
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
}
