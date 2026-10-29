// Package workload describes pings independently of hardware assembly.
package workload

import (
	"github.com/sarchlab/akita/v5/examples/ping"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec names the hardware contract and the ping schedule.
type Spec struct {
	Agent, Peer string
	SendAt      []timing.VTimeInPicoSec
}

func DefaultSpec() Spec {
	return Spec{Agent: "AgentA", Peer: "AgentB", SendAt: []timing.VTimeInPicoSec{1, 3}}
}

type Workload struct{ spec Spec }

func New(spec Spec) *Workload {
	spec.SendAt = append([]timing.VTimeInPicoSec(nil), spec.SendAt...)
	return &Workload{spec: spec}
}

// Execute loads the schedule using the ping component's own API. The contract
// is two named *ping.Comp values with bound Out ports, after Initialize.
func (w *Workload) Execute(s *sim.Simulation) {
	agent := s.Lookup(w.spec.Agent).(*ping.Comp)
	peer := s.Lookup(w.spec.Peer).(*ping.Comp)
	for _, at := range w.spec.SendAt {
		ping.SchedulePing(agent, at, peer.Ports.Out.AsRemote())
	}
}
