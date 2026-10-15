package datamover

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq: 1 * timing.GHz,
	}
}

func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	*initialMiddlewares = middlewares{
		Ctrl:      &ctrlMiddleware{comp: c},
		CtrlParse: &ctrlParseMW{comp: c},
		DataTransfer: &dataTransferMW{
			comp: c,
		},
	}
}

func validateSpec(spec Spec) error {
	if err := spec.Inside.Validate(); err != nil {
		return err
	}
	if err := spec.Outside.Validate(); err != nil {
		return err
	}
	return nil
}
