package rob

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/internal/memcontroltest"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontroltest.Harness {
		sim := akitasim.MakeBuilder().Build()

		spec := DefaultSpec()
		spec.Bottom = messaging.RemotePort("BottomUnit")

		port := func(name string) messaging.Port {
			p := twowaybuffered.MakeBuilder().
				WithSimulation(sim).
				WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
				Build()
			return p
		}

		comp := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("ROB")

		comp.BindPort("Top", port("Top"))
		comp.BindPort("Bottom", port("Bottom"))
		comp.BindPort("Control", port("Control"))
		for _, p := range []messaging.Port{comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control} {
			(&noopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontroltest.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return len(comp.State.Transactions) == 0
			},
		}
	}

	memcontroltest.RunContract(t, "rob", build, memcontroltest.Universal())
}

var _ memcontroltest.Controllable = (*Comp)(nil)
