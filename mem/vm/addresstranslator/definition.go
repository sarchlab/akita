package addresstranslator

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:           1 * timing.GHz,
		NumReqPerCycle: 4,
		Log2PageSize:   12,
		DeviceID:       1,
	}
}

func (m *middlewares) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	*m = middlewares{
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
