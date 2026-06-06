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
	agentMem := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(agent).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Mem")
	agent.AssignPort("Mem", agentMem)
	createProgressBars(s, agent)

	dramSpec := idealmemcontroller.DefaultSpec()
	dramSpec.Capacity = 4 * mem.GB
	dram := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(dramSpec).
		Build("DRAM")
	dramTop := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(dram).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Top")
	dram.AssignPort("Top", dramTop)
	dramCtrl := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(dram).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Control")
	dram.AssignPort("Control", dramCtrl)

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
	cacheTop := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(writeBackCache).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Top")
	writeBackCache.AssignPort("Top", cacheTop)
	cacheBottom := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(writeBackCache).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Bottom")
	writeBackCache.AssignPort("Bottom", cacheBottom)
	cacheControl := modeling.MakePortBuilder().
		WithRegistrar(s).
		WithComponent(writeBackCache).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Control")
	writeBackCache.AssignPort("Control", cacheControl)

	agent.LowModule = writeBackCache.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(writeBackCache.GetPortByName("Bottom"))
	conn.PlugIn(writeBackCache.GetPortByName("Top"))
	conn.PlugIn(dram.GetPortByName("Top"))

	return s, engine, agent
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
