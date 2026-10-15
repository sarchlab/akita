package rob

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		sim := akitasim.MakeBuilder().Build()

		spec := Definition.DefaultSpec
		spec.Bottom = messaging.RemotePort("BottomUnit")

		port := func(name string) messaging.Port {
			p := twowaybuffered.NewPort(16, 16)
			return p
		}

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
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

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return len(comp.State.Transactions) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "rob", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
