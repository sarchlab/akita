package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/internal/accessworkload"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 100000, "Number of accesses to generate")
var maxAddressFlag = flag.Uint64("max-address", 1048576, "Address range to use")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")
var traceFlag = flag.Bool("trace", false, "Collect trace")

type platformSpec struct{ MemoryCapacity uint64 }

func declarePlatform(spec platformSpec, bp *blueprint.Blueprint) {
	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.ReadLeft, agentSpec.WriteLeft = 0, 0
	agentSpec.LowModule = bp.PortRef("Cache.Top")
	bp.Add("MemAccessAgent", blueprint.Comp[*memaccessagent.Comp](agentSpec).Port("Mem", bufferSpec()))
	dramStorage := bp.Add("DRAMStorage", blueprint.Res(mem.StorageSpec{Capacity: spec.MemoryCapacity,
		UnitSize: 4 * mem.KB}))
	bp.Add("DRAM", blueprint.Comp[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).Use("Storage",
		dramStorage).Port("Top", bufferSpec()).Port("Control", bufferSpec()))
	cacheSpec := writeback.DefaultSpec()
	cacheSpec.NumSets = 64
	cacheSpec.Log2BlockSize = 6
	cacheSpec.NumWays = 4
	cacheSpec.NumMSHREntries = 4
	cacheSpec.NumReqPerCycle = 16
	cacheSpec.DirectoryWidth = 16
	cacheSpec.BankWidth = 16
	cacheSpec.Bottom.Destinations = []messaging.RemotePort{bp.PortRef("DRAM.Top")}
	storage := bp.Add("CacheStorage", blueprint.Res(mem.StorageSpec{Capacity: cacheSpec.ByteSize(),
		UnitSize: 4 * mem.KB}))
	bp.Add("Cache", blueprint.Comp[*writeback.Comp](cacheSpec).Use("Storage", storage).Port("Top",
		bufferSpec()).Port("Bottom", bufferSpec()).Port("Control", bufferSpec()))
	// Retain the existing arbitration order exactly.
	bp.Connect("Conn", timing.GHz, bp.PortRef("MemAccessAgent.Mem"), bp.PortRef("Cache.Bottom"), bp.PortRef("Cache.Top"),
		bp.PortRef("DRAM.Top"))
}

func bufferSpec() twowaybuffered.Spec {
	return twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
}

func buildEnvironment(seed int64) (*sim.Simulation, *memaccessagent.Comp) {
	monitor := monitoring.MakeBuilder().Build()
	b := sim.MakeBuilder().WithMonitor(monitor)
	if *parallelFlag {
		b = b.WithParallelEngine()
	}
	if *traceFlag {
		b = b.WithVisTracingOnStart()
	}
	s := b.Build()
	s.Add("", blueprint.NewDomain[platformSpec, modeling.None](platformSpec{MemoryCapacity: 4 * mem.GB}, declarePlatform))
	if err := s.Initialize(); err != nil {
		panic(err)
	}
	work := accessworkload.DefaultSpec()
	work.Seed, work.ReadCount, work.WriteCount = seed, *numAccessFlag, *numAccessFlag
	work.AddressRange = *maxAddressFlag
	accessworkload.New(work).Execute(s)
	agent := s.Lookup("MemAccessAgent").(*memaccessagent.Comp)
	memaccessagent.SetProgressTrackers(agent, monitor.CreateProgressBar(agent.Name()+".Writes",
		uint64(agent.State.WriteLeft)), monitor.CreateProgressBar(agent.Name()+".Reads", uint64(agent.State.ReadLeft)))
	return s, agent
}

func main() {
	flag.Parse()
	seed := *seedFlag
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)
	s, a := buildEnvironment(seed)
	if err := s.Run(); err != nil {
		panic(err)
	}
	if len(a.State.PendingWriteReq) > 0 || len(a.State.PendingReadReq) > 0 {
		panic("Not all req returned")
	}
	if a.State.WriteLeft > 0 || a.State.ReadLeft > 0 {
		panic("more requests to send")
	}
	if err := s.Terminate(); err != nil {
		panic(err)
	}
}
