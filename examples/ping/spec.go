package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// PingSpec is the immutable configuration for a ping component.
type PingSpec struct {
	OutPort messaging.Port
}
