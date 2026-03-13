package arbitration

import "github.com/sarchlab/akita/v5/queueing"

// Arbiter can determine which buffer can send a message out
type Arbiter interface {
	// Add a buffer for arbitration
	AddBuffer(buf queueing.BufferState)

	// Arbitrate returns a set of ports that can send request in the next cycle.
	Arbitrate() []queueing.BufferState
}
