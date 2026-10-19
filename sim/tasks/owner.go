package tasks

import "github.com/sarchlab/akita/v5/sim/naming"

// Owner identifies the object that started a task. Runtime services belong to
// the simulation's Tracker, not to the owner.
type Owner interface{ naming.Named }
