package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the address translator satisfies the
// universal control verbs. Invalidate and Flush must respond as
// unsupported — the translator holds no cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()
		spec := DefaultSpec()
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

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(resources).
			Build("AddressTranslator")

		for _, name := range []string{"Top", "Bottom", "Translation", "Control"} {
			conn := &noopConn{}
			conn.PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
		}
	}

	control.RunContract(t, "addresstranslator", build, control.Universal())
}

var _ control.Controllable = (*Comp)(nil)
