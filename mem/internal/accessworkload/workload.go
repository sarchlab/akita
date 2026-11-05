// Package accessworkload loads one random memory-access workload onto an agent.
package accessworkload

import (
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/sim"
)

// Spec describes work independently of hardware assembly.
type Spec struct {
	AgentName                   string
	ReadCount, WriteCount       int
	AddressRange, AddressOffset uint64
	Seed                        int64
}

func DefaultSpec() Spec {
	return Spec{AgentName: "MemAccessAgent", ReadCount: 1000, WriteCount: 1000,
		AddressRange: 1048576, AddressOffset: 0, Seed: 1}
}

type Workload struct{ spec Spec }

func New(spec Spec) *Workload { return &Workload{spec: spec} }

// Execute requires the named memaccessagent.Comp in the initialized simulation.
// Lookup/type assertion and the agent's own Start validate that contract.
func (w *Workload) Execute(s *sim.Simulation) {
	a := s.Lookup(w.spec.AgentName).(*memaccessagent.Comp)
	memaccessagent.Start(a, memaccessagent.Accesses{ReadCount: w.spec.ReadCount, WriteCount: w.spec.WriteCount,
		AddressRange: w.spec.AddressRange, AddressOffset: w.spec.AddressOffset, Seed: w.spec.Seed})
}
