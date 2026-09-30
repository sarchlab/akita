package rob

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)

		spec := Definition.DefaultSpec
		spec.BottomUnit = messaging.RemotePort("BottomUnit")

		port := func(name string) messaging.Port {
			p := messaging.NewPort(nil, 16, 16, "ROB."+name)
			(&noopConn{}).PlugIn(p)
			return p
		}

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithPorts(Ports{
				Top:     port("Top"),
				Bottom:  port("Bottom"),
				Control: port("Control"),
			}).
			Build("ROB")

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
