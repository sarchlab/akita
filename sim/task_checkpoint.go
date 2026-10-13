package sim

import "github.com/sarchlab/akita/v5/sim/tasks"

// The simulation owns the reserved entity name and optional archive membership.
const taskTrackerCheckpointName = "TaskTracker"

// taskTrackerCheckpoint adapts the tracker to the simulation entity inventory.
// Persistence belongs to tasks; archive naming and registration belong to sim.
type taskTrackerCheckpoint struct{ *tasks.Tracker }

func (*taskTrackerCheckpoint) Name() string { return taskTrackerCheckpointName }
