// This acceptance program instantiates four private agent-chain domains above
// a shared L2 cache, L2 TLB, memory controller, and MMU. Agents access disjoint
// address ranges; shared connections retain their original arbitration order.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/sharedl2vm/agentchain"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

const numAgents = 4
const ptBase = uint64(0x100000)

var seedFlag = flag.Int64("seed", 0, "Random Seed")
var numAccessFlag = flag.Int("num-access", 1000, "Number of accesses per agent")
var maxAddressFlag = flag.Uint64("max-address", 16*mem.MB, "Per-agent memory range size")
var traceFlag = flag.Bool("trace", false, "Collect trace")
var parallelFlag = flag.Bool("parallel", false, "Test with parallel engine")

type agentChain struct{ agent *memaccessagent.Comp }
type platformSpec struct {
	Seed       int64
	NumAccess  int
	MaxAddress uint64
}

func agentStride() uint64             { return stride(*maxAddressFlag) }
func stride(maxAddress uint64) uint64 { return (maxAddress + 3) / 4 * 4 }
func buffers() twowaybuffered.Spec {
	return twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
}
func declarePlatform(spec platformSpec, bp *blueprint.Blueprint) {
	memory := bp.Add("MemCtrlStorage",
		blueprint.Res(mem.StorageSpec{Capacity: ptBase + numAgents*stride(spec.MaxAddress) + mem.MB,
			UnitSize: 4 * mem.KB}))
	mcSpec := idealmemcontroller.DefaultSpec()
	mcSpec.NumReqPerCycle = 1
	mcSpec.Latency = 100
	bp.Add("MemCtrl", blueprint.Comp[*idealmemcontroller.Comp](mcSpec).Use("Storage", memory).Port("Top",
		buffers()).Port("Control", buffers()))
	cacheSpec := writeback.DefaultSpec()
	cacheSpec.NumWays = 4
	cacheSpec.NumReqPerCycle = 2
	cacheSpec.DirectoryWidth = 2
	cacheSpec.BankWidth = 2
	cacheSpec.Bottom.Destinations = []messaging.RemotePort{bp.PortRef("MemCtrl.Top")}
	storage := bp.Add("L2CacheStorage", blueprint.Res(mem.StorageSpec{Capacity: cacheSpec.ByteSize(),
		UnitSize: 4 * mem.KB}))
	bp.Add("L2Cache", blueprint.Comp[*writeback.Comp](cacheSpec).Use("Storage", storage).Port("Top",
		buffers()).Port("Bottom", buffers()).Port("Control", buffers()))
	pt := bp.Add("PageTable", blueprint.Res(vm.PageTableSpec{Log2PageSize: 12}))
	mmuSpec := mmu.DefaultSpec()
	mmuSpec.Log2PageSize = 12
	mmuSpec.MaxInFlightTranslations = 16
	mmuSpec.Latency = 10
	bp.Add("IoMMU", blueprint.Comp[*mmu.Comp](mmuSpec).Use("PageTable", pt).Port("Top", buffers()).
		Port("Control", buffers()))
	tlbSpec := tlb.DefaultSpec()
	tlbSpec.NumWays = 64
	tlbSpec.NumSets = 64
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 4
	tlbSpec.Bottom.Destinations = []messaging.RemotePort{bp.PortRef("IoMMU.Top")}
	bp.Add("L2TLB", blueprint.Comp[*tlb.Comp](tlbSpec).Port("Top", buffers()).Port("Bottom", buffers()).
		Port("Control", buffers()))
	declareAgentChains(spec, bp)
}
func declareAgentChains(spec platformSpec, bp *blueprint.Blueprint) {
	memoryPorts := []messaging.RemotePort{bp.PortRef("L2Cache.Top")}
	translationPorts := []messaging.RemotePort{bp.PortRef("L2TLB.Top")}
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("Chain[%d]", i)
		chainSpec := agentchain.DefaultSpec()
		chainSpec.Seed = spec.Seed + int64(i)
		chainSpec.NumAccess = spec.NumAccess
		chainSpec.MaxAddress = spec.MaxAddress
		chainSpec.AddressOffset = uint64(i) * stride(spec.MaxAddress)
		chainSpec.Memory = bp.PortRef("L2Cache.Top")
		chainSpec.Translation = bp.PortRef("L2TLB.Top")
		bp.Add(name, agentchain.Node(chainSpec))
		memoryPorts = append(memoryPorts, bp.PortRef(name+".Memory"))
		translationPorts = append(translationPorts, bp.PortRef(name+".Translation"))
	}
	bp.Connect("ConnL1L2", timing.GHz, memoryPorts...)
	bp.Connect("ConnL1L2TLB", timing.GHz, translationPorts...)
	bp.Connect("ConnL2Mem", timing.GHz, bp.PortRef("L2Cache.Bottom"), bp.PortRef("MemCtrl.Top"))
	bp.Connect("ConnL2TLBMMU", timing.GHz, bp.PortRef("L2TLB.Bottom"), bp.PortRef("IoMMU.Top"))
}

func setupTest(seed int64) (*sim.Simulation, timing.Engine, []agentChain) {
	monitor := monitoring.MakeBuilder().Build()
	b := sim.MakeBuilder().WithMonitor(monitor)
	if *parallelFlag {
		b = b.WithParallelEngine()
	}
	if *traceFlag {
		b = b.WithVisTracingOnStart()
	}
	s := b.Build()
	s.Add("", blueprint.NewDomain[platformSpec, modeling.None](platformSpec{seed, *numAccessFlag,
		*maxAddressFlag}, declarePlatform))
	if err := s.Initialize(); err != nil {
		panic(err)
	}
	pt := blueprint.Lookup[*vm.PageTable](s, "PageTable")
	setupPageTable(numAgents*agentStride(), pt)
	chains := make([]agentChain, numAgents)
	for i := range chains {
		a := blueprint.Lookup[*memaccessagent.Comp](s, fmt.Sprintf("Chain[%d].Agent", i))
		chains[i] = agentChain{a}
		memaccessagent.SetProgressTrackers(a, monitor.CreateProgressBar(a.Name()+".Writes",
			uint64(a.State.WriteLeft)), monitor.CreateProgressBar(a.Name()+".Reads", uint64(a.State.ReadLeft)))
	}
	return s, s.Engine(), chains
}
func setupPageTable(addressRange uint64, pt *vm.PageTable) {
	const pageSize = uint64(4096)
	numEntries := (addressRange-1)/pageSize + 1
	for i := uint64(0); i < numEntries; i++ {
		vAddr := i * pageSize
		pt.Insert(vm.Page{PID: 1, VAddr: vAddr, PAddr: ptBase + vAddr, PageSize: pageSize, Valid: true})
	}
}
func main() {
	flag.Parse()
	seed := *seedFlag
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	fmt.Fprintf(os.Stderr, "Seed %d\n", seed)
	s, e, chains := setupTest(seed)
	for _, c := range chains {
		c.agent.TickLater()
	}
	if err := e.Run(); err != nil {
		panic(err)
	}
	for i, c := range chains {
		if len(c.agent.State.PendingWriteReq) > 0 || len(c.agent.State.PendingReadReq) > 0 {
			panic(fmt.Sprintf("agent %d: not all req returned", i))
		}
		if c.agent.State.WriteLeft > 0 || c.agent.State.ReadLeft > 0 {
			panic(fmt.Sprintf("agent %d: more requests to send", i))
		}
	}
	fmt.Fprintf(os.Stderr, "All %d agents completed %d reads and %d writes each.\n", numAgents,
		*numAccessFlag, *numAccessFlag)
	if err := s.Terminate(); err != nil {
		panic(err)
	}
}
