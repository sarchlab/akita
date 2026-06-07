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
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
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

func buildEnvironment() (*simulation.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
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
	assignPorts(s, agent, "Mem")
	createProgressBars(s, agent)

	dram := buildDRAM(s)

	addressToPortMapper := new(mem.SinglePortMapper)
	addressToPortMapper.Port = dram.GetPortByName("Top").AsRemote()

	cacheSpec := writeback.DefaultSpec()
	cacheSpec.TotalByteSize = 16 * mem.KB
	cacheSpec.Log2BlockSize = 6
	cacheSpec.WayAssociativity = 4
	cacheSpec.NumMSHREntry = 4
	cacheSpec.NumReqPerCycle = 16
	writeBackCache := writeback.MakeBuilder().
		WithRegistrar(s).
		WithSpec(cacheSpec).
		WithResources(writeback.Resources{
			AddressToPortMapper: addressToPortMapper,
		}).
		Build("Cache")
	assignPorts(s, writeBackCache, "Top", "Bottom", "Control")

	agent.LowModule = writeBackCache.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(writeBackCache.GetPortByName("Bottom"))
	conn.PlugIn(writeBackCache.GetPortByName("Top"))
	conn.PlugIn(dram.GetPortByName("Top"))

	return s, engine, agent
}

// buildDRAM builds and registers the backing ideal memory controller.
func buildDRAM(s *simulation.Simulation) *idealmemcontroller.Comp {
	dramSpec := idealmemcontroller.DefaultSpec()
	dramSpec.Capacity = 4 * mem.GB
	dram := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(dramSpec).
		Build("DRAM")
	assignPorts(s, dram, "Top", "Control")

	return dram
}

// assignPorts builds a port for each declared name on the component and assigns
// it, choosing a default buffer size.
func assignPorts(
	s *simulation.Simulation,
	comp messaging.Component,
	names ...string,
) {
	for _, name := range names {
		p := modeling.MakePortBuilder().
			WithRegistrar(s).
			WithComponent(comp).
			WithSpec(modeling.PortSpec{BufSize: 16}).
			Build(name)
		comp.AssignPort(name, p)
	}
}

func createProgressBars(
	s *simulation.Simulation,
	agent *memaccessagent.MemAccessAgent,
) {
	if monitor := s.GetMonitor(); monitor != nil {
		agent.CreateProgressBars(monitor.CreateProgressBar)
	}
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
