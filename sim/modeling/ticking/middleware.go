package ticking

import "github.com/sarchlab/akita/v5/sim/timing"

// Middleware handles events in field declaration order. It reports whether it
// changed state or sent or received a message.
// A component serializes its handlers, even under the parallel engine.
type Middleware interface{ Handle(timing.Event) bool }
