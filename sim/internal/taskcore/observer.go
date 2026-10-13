package taskcore

// Observer receives resolved task notifications. Callbacks for a given task
// are ordered; callbacks for different tasks may be concurrent. Observers own
// their synchronization and retention. Callbacks must not call ports or Tracker
// methods, including the component helpers. No history is replayed.
type Observer interface {
	TaskStarted(Task)
	TaskEnded(Task)
	TagAdded(Tag)
	MilestoneAdded(Milestone)
}

// NopObserver can be embedded to implement only callbacks of interest.
type NopObserver struct{}

func (NopObserver) TaskStarted(Task)         {}
func (NopObserver) TaskEnded(Task)           {}
func (NopObserver) TagAdded(Tag)             {}
func (NopObserver) MilestoneAdded(Milestone) {}
