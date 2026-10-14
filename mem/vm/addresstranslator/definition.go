package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem"
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
		TranslationMapperType: mem.AddressMapperSingle,
		MemMapperType:         mem.AddressMapperSingle,
		Freq:                  1 * timing.GHz,
		NumReqPerCycle:        4,
		Log2PageSize:          12,
		DeviceID:              1,
	},
	ValidateSpec:   validateSpec,
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) middlewares {
	return middlewares{
		Ctrl:            &ctrlMiddleware{comp: c},
		ParseTranslate:  &parseTranslateMW{comp: c, translationProvider: c.Spec.translationMapping().Mapper()},
		RespondPipeline: &respondPipelineMW{comp: c, memProvider: c.Spec.memMapping().Mapper()},
	}
}

func validateSpec(spec Spec) error {
	if err := spec.memMapping().Validate(); err != nil {
		return err
	}
	if err := spec.translationMapping().Validate(); err != nil {
		return err
	}
	return nil
}
