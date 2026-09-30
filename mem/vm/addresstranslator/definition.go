package addresstranslator

import (
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the AddressTranslator, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec: Spec{
		Freq:           1 * timing.GHz,
		NumReqPerCycle: 4,
		Log2PageSize:   12,
		DeviceID:       1,
	},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{
		Ctrl:            &ctrlMiddleware{comp: c},
		ParseTranslate:  &parseTranslateMW{comp: c},
		RespondPipeline: &respondPipelineMW{comp: c},
	}
}
