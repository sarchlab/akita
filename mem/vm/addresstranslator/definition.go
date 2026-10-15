package addresstranslator

import (
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the AddressTranslator, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:           1 * timing.GHz,
		NumReqPerCycle: 4,
		Log2PageSize:   12,
		DeviceID:       1,
	},
}

func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(err)
	}

	*initialMiddlewares = middlewares{
		Ctrl:            &ctrlMiddleware{comp: c},
		ParseTranslate:  &parseTranslateMW{comp: c},
		RespondPipeline: &respondPipelineMW{comp: c},
	}
}

func validateSpec(spec Spec) error {
	if err := spec.Bottom.Validate(); err != nil {
		return err
	}
	if err := spec.Translation.Validate(); err != nil {
		return err
	}
	return nil
}
