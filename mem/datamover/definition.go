package datamover

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		MaxTransactions: 1,
		Freq:            1 * timing.GHz,
	}
}

func (m *middlewares) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	*m = middlewares{
		Ctrl:      &ctrlMiddleware{comp: c},
		CtrlParse: &ctrlParseMW{comp: c},
		DataTransfer: &dataTransferMW{
			comp: c,
		},
	}
}

func validateSpec(spec Spec) error {
	if spec.MaxTransactions <= 0 {
		return fmt.Errorf("MaxTransactions must be positive, got %d", spec.MaxTransactions)
	}
	if err := spec.Inside.Validate(); err != nil {
		return err
	}
	if err := spec.Outside.Validate(); err != nil {
		return err
	}
	if err := validateRoutingAlignment("inside", spec.Inside, spec.InsideByteGranularity); err != nil {
		return err
	}
	return validateRoutingAlignment("outside", spec.Outside, spec.OutsideByteGranularity)
}

// validateRoutingAlignment requires the active mapper boundaries to align to
// the side's granularity, because each request is routed by its aligned unit
// base and must not straddle a boundary.
func validateRoutingAlignment(side string, m mem.AddressMapper, granularity uint64) error {
	if granularity == 0 {
		return nil
	}
	if len(m.Destinations) > 1 && m.InterleavingSize%granularity != 0 ||
		m.RangeHigh != 0 && (m.RangeLow%granularity != 0 || m.RangeHigh%granularity != 0) {
		return fmt.Errorf("%s routing boundaries must align to %d bytes", side, granularity)
	}
	return nil
}
