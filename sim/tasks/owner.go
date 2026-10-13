package tasks

import (
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Owner supplies the task service and clock of one simulation. ComponentBase
// implements Owner without requiring any task hooks. The clock, ID allocator,
// and tracker must belong to the same simulation.
type Owner interface {
	naming.Named
	timing.TimeTeller
	NewID() uint64
	TaskTracker() *Tracker
}
