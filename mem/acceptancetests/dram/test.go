package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access",
	100000, "Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")
var traceFlag = flag.Bool("trace", false, "Collect trace")

func setupTest(seed int64) (*sim.Simulation, timing.Engine, *memaccessagent.MemAccessAgent) {
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

	conn := direct.NewConnection("Conn", s, timing.GHz)

	// The agent sends to the memory controller's Top port, so the
	// controller's ports are created before the agent is built.
	memCtrlPorts := dram.Ports{
		Top:     twowaybuffered.NewPort("Mem.Top", 16, 16),
		Control: twowaybuffered.NewPort("Mem.Control", 16, 16),
	}

	agentSpec := memaccessagent.Definition.DefaultSpec
	agentSpec.MaxAddress = *maxAddressFlag
	agentSpec.WriteLeft = *numAccessFlag
	agentSpec.ReadLeft = *numAccessFlag
	agentSpec.RandSeed = seed

	agent := memaccessagent.Definition.Builder().
		WithSimulation(s).
		WithSpec(agentSpec).
		WithResources(memaccessagent.Resources{LowModule: memCtrlPorts.Top}).
		WithPorts(memaccessagent.Ports{
			Mem: twowaybuffered.NewPort("MemAccessAgent.Mem", 16, 16),
		}).
		Build("MemAccessAgent")
	memaccessagent.SetProgressTrackers(agent,
		monitor.CreateProgressBar(agent.Name()+".Writes", uint64(agent.State.WriteLeft)),
		monitor.CreateProgressBar(agent.Name()+".Reads", uint64(agent.State.ReadLeft)),
	)

	dramSpec := dram.Definition.DefaultSpec
	dramSpec.Freq = 1 * timing.GHz

	memCtrl := dram.Definition.Builder().
		WithSimulation(s).
		WithSpec(dramSpec).
		WithResources(dram.Resources{
			Storage: mem.MakeStorageBuilder().
				WithCapacity(dramCapacity(dramSpec)).
				WithSimulation(s).
				Build("Mem.Storage"),
		}).
		WithPorts(memCtrlPorts).
		Build("Mem")

	conn.PlugIn(agent.Ports.Mem)
	conn.PlugIn(memCtrl.Ports.Top)

	return s, engine, agent
}

// dramCapacity returns the number of bytes addressable by a DRAM with the
// given geometry.
func dramCapacity(spec dram.Spec) uint64 {
	devicePerRank := spec.BusWidth / spec.DeviceWidth
	bankSize := spec.NumCol * spec.NumRow * spec.DeviceWidth / 8
	rankSize := bankSize * spec.NumBank * devicePerRank
	totalSize := rankSize * spec.NumRank * spec.NumChannel

	return uint64(totalSize)
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
