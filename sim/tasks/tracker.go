package tasks

import "github.com/sarchlab/akita/v5/sim/internal/taskcore"

// Tracker owns task lifecycles and observer subscriptions for one simulation.
// Enable it with sim.Builder.WithTaskTracking and access it through Simulation.Tasks.
// Subscribe registers observers before the first engine run. An empty owner
// scope observes the whole simulation; each owner scope includes its ports.
// Framework construction, port wiring, and checkpointing are internal.
type Tracker = taskcore.Tracker
