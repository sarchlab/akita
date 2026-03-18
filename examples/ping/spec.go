package ping

import "github.com/sarchlab/akita/v5/sim"

// PingSpec is the immutable configuration for a ping component.
type PingSpec struct {
	OutPort sim.Port
}
