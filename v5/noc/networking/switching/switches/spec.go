package switches

import (
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the switch.
type Spec struct {
	Freq sim.Freq `json:"freq"`
}
