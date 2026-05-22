package switches

import (
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the switch.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}
