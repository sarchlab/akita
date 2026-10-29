// Package agentchain declares one private agent, ROB, address translator, L1
// cache and L1 TLB chain. The parent supplies the shared lower-level addresses.
package agentchain

import (
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/rob"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec configures one chain. Parent-supplied addresses are already absolute.
type Spec struct {
	Seed          int64
	NumAccess     int
	MaxAddress    uint64
	AddressOffset uint64
	Memory        messaging.RemotePort
	Translation   messaging.RemotePort
}

// DefaultSpec returns an independent default configuration; the parent supplies destinations.
func DefaultSpec() Spec {
	return Spec{Seed: 1, NumAccess: 1000, MaxAddress: 16777216, AddressOffset: 0, Memory: "", Translation: ""}
}

// Ports exposes the two links to the shared hierarchy.
type Ports struct {
	Memory      messaging.Port
	Translation messaging.Port
}

// Node declares the chain without creating runtime objects.
func Node(spec Spec) *blueprint.DomainNode { return blueprint.NewDomain[Spec, Ports](spec, build) }
func buffers() twowaybuffered.Spec {
	return twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
}
func build(spec Spec, bp *blueprint.Blueprint) {
	cacheSpec := writethrough.DefaultSpec()
	cacheSpec.WritePolicy = "write-through"
	cacheSpec.NumWays = 2
	cacheSpec.NumSets = 32
	cacheSpec.Bottom.Destinations = []messaging.RemotePort{spec.Memory}
	storage := bp.Add("L1Cache.Storage", blueprint.Res(mem.StorageSpec{Capacity: cacheSpec.ByteSize(),
		UnitSize: 4 * mem.KB}))
	bp.Add("L1Cache", blueprint.Comp[*writethrough.Comp](cacheSpec).Use("Storage", storage).Port("Top",
		buffers()).Port("Bottom", buffers()).Port("Control", buffers()))
	tlbSpec := tlb.DefaultSpec()
	tlbSpec.NumWays = 8
	tlbSpec.NumSets = 8
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 2
	tlbSpec.Bottom.Destinations = []messaging.RemotePort{spec.Translation}
	bp.Add("L1TLB", blueprint.Comp[*tlb.Comp](tlbSpec).Port("Top", buffers()).Port("Bottom", buffers()).
		Port("Control", buffers()))
	atSpec := addresstranslator.DefaultSpec()
	atSpec.Log2PageSize = 12
	atSpec.NumReqPerCycle = 4
	atSpec.Bottom.Destinations = []messaging.RemotePort{bp.PortRef("L1Cache.Top")}
	atSpec.Translation.Destinations = []messaging.RemotePort{bp.PortRef("L1TLB.Top")}
	bp.Add("AT", blueprint.Comp[*addresstranslator.Comp](atSpec).Port("Top", buffers()).Port("Bottom",
		buffers()).Port("Translation", buffers()).Port("Control", buffers()))
	robSpec := rob.DefaultSpec()
	robSpec.NumReqPerCycle = 4
	robSpec.Bottom = bp.PortRef("AT.Top")
	bp.Add("ROB", blueprint.Comp[*rob.Comp](robSpec).Port("Top", buffers()).Port("Bottom", buffers()).
		Port("Control", buffers()))
	agentSpec := memaccessagent.DefaultSpec()
	agentSpec.MaxAddress = spec.MaxAddress
	agentSpec.AddressOffset = spec.AddressOffset
	agentSpec.ReadLeft = spec.NumAccess
	agentSpec.WriteLeft = spec.NumAccess
	agentSpec.RandSeed = spec.Seed
	agentSpec.LowModule = bp.PortRef("ROB.Top")
	bp.Add("Agent", blueprint.Comp[*memaccessagent.Comp](agentSpec).Port("Mem", buffers()))
	bp.Connect("ConnAgentROB", timing.GHz, bp.PortRef("Agent.Mem"), bp.PortRef("ROB.Top"))
	bp.Connect("ConnROBAT", timing.GHz, bp.PortRef("ROB.Bottom"), bp.PortRef("AT.Top"))
	bp.Connect("ConnATL1", timing.GHz, bp.PortRef("AT.Bottom"), bp.PortRef("L1Cache.Top"))
	bp.Connect("ConnATTrans", timing.GHz, bp.PortRef("AT.Translation"), bp.PortRef("L1TLB.Top"))
	bp.Expose("Memory", "L1Cache.Bottom")
	bp.Expose("Translation", "L1TLB.Bottom")
}
