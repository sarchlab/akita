package directconnection

import (
	"github.com/sarchlab/akita/v5/timing"
)

// Spec holds immutable configuration for the DirectConnection.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}
