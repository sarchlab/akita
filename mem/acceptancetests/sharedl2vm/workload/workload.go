// Package workload exercises four agent chains through a shared virtual memory hierarchy.
package workload

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/mem/internal/accessworkload"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// Spec is input data; hardware storage capacity is configured independently.
type Spec struct {
	ReadCount, WriteCount       int
	AddressRange, AddressOffset uint64
	Seed                        int64
}

func DefaultSpec() Spec {
	return Spec{ReadCount: 1000, WriteCount: 1000, AddressRange: 16777216, AddressOffset: 0, Seed: 1}
}

type Workload struct{ spec Spec }

func New(spec Spec) *Workload { return &Workload{spec: spec} }

// Execute uses the PageTable resource and Chain[0..3].Agent hardware contract.
// Virtual pages map at physical base 0x100000; the hardware must provide enough
// memory for these pages. It then loads and starts each agent's private stream.
func (w *Workload) Execute(s *sim.Simulation) {
	pt := s.Lookup("PageTable").(*vm.PageTable)
	const physicalBase = uint64(0x100000)
	if w.spec.AddressRange < 4 || w.spec.AddressRange > math.MaxUint64-3 || w.spec.AddressOffset%4 != 0 {
		panic("sharedl2vm workload: invalid aligned address range")
	}
	stride := (w.spec.AddressRange + 3) / 4 * 4
	if stride > (math.MaxUint64-w.spec.AddressOffset)/4 {
		panic("sharedl2vm workload: address range overflows")
	}
	end := w.spec.AddressOffset + 4*stride
	if end > math.MaxUint64-physicalBase {
		panic("sharedl2vm workload: physical address range overflows")
	}
	for address := w.spec.AddressOffset / 4096 * 4096; address < end; address += 4096 {
		pt.Insert(vm.Page{PID: 1, VAddr: address, PAddr: physicalBase + address, PageSize: 4096, Valid: true})
	}
	for i := 0; i < 4; i++ {
		spec := accessworkload.DefaultSpec()
		spec.AgentName = fmt.Sprintf("Chain[%d].Agent", i)
		spec.ReadCount, spec.WriteCount = w.spec.ReadCount, w.spec.WriteCount
		spec.AddressRange = w.spec.AddressRange
		spec.AddressOffset = w.spec.AddressOffset + uint64(i)*stride
		spec.Seed = w.spec.Seed + int64(i)
		accessworkload.New(spec).Execute(s)
	}
}
