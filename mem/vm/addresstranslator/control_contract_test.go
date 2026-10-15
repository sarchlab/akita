package addresstranslator

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// TestControlContract verifies the address translator satisfies the
// universal control verbs. Invalidate and Flush must respond as
// unsupported — the translator holds no cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		sim := akitasim.MakeBuilder().Build()

		spec := Definition.DefaultSpec
		spec.Log2PageSize = 12
		spec.Freq = 1

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("MemPort")}
		spec.Translation.Destinations = []messaging.RemotePort{messaging.RemotePort("TranslationProvider")}

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			Build("AddressTranslator")
		compPorts := makePorts("AddressTranslator", topBufSize)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Translation", compPorts.Translation)
		comp.BindPort("Control", compPorts.Control)

		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom,
			comp.Ports.Translation, comp.Ports.Control,
		} {
			conn := &noopConn{}
			conn.BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return len(comp.State.Transactions) == 0 &&
					len(comp.State.InflightReqToBottom) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "addresstranslator", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
