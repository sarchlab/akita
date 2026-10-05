package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// TestControlContract verifies the address translator satisfies the
// universal control verbs. Invalidate and Flush must respond as
// unsupported — the translator holds no cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		spec := Definition.DefaultSpec
		spec.Log2PageSize = 12
		spec.Freq = 1

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(resources).
			WithPorts(makePorts("AddressTranslator", topBufSize)).
			Build("AddressTranslator")

		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom,
			comp.Ports.Translation, comp.Ports.Control,
		} {
			conn := &noopConn{}
			conn.PlugIn(p)
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
