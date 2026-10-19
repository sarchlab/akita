package event

import "github.com/sarchlab/akita/v5/sim/timing"

// Middleware handles events in field declaration order. It schedules subsequent work explicitly.
// A component serializes its handlers, even under the parallel engine.
type Middleware interface{ Handle(timing.Event) }
