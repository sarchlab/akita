package tasks

import "github.com/sarchlab/akita/v5/sim/internal/taskcore"

// Observer receives resolved task notifications. Callbacks for a given task
// are ordered; callbacks for different tasks may be concurrent. Observers own
// their synchronization and retention. Callbacks must not call ports or Tracker
// methods, including the component helpers. No history is replayed.
type Observer = taskcore.Observer

// NopObserver can be embedded to implement only callbacks of interest.
type NopObserver = taskcore.NopObserver
