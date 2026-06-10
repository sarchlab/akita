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

		spec := DefaultSpec()
		spec.BottomUnit = messaging.RemotePort("BottomUnit")

		reg := modeling.NewStandaloneRegistrar(engine)

		comp := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("ROB")

		for _, name := range []string{"Top", "Bottom", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(comp).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			comp.AssignPort(name, p)
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return len(comp.State.Transactions) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "rob", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
