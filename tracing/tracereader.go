package tracing

// TaskQuery is used to define the tasks to be queried. Not all the field has to
// be set. If the fields are empty, the criteria is ignored.
type TaskQuery struct {
	// Use ID to select a single task by its ID.
	ID string

	// Use ParentID to select all the tasks that are children of a task.
	ParentID string

	// Use Kind to select all the tasks that are of a kind.
	Kind string

	// Use Where to select all the tasks that are executed at a location.
	Where string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select tasks that overlaps with the given task range.
	StartTime, EndTime float64

	// EnableParentTask will also query the parent task of the selected tasks.
	EnableParentTask bool
}

type DelayQuery struct {
	// Use EventID to select a single delay event by its ID.
	EventID string

	// Use TaskID to select all delay events associated with a task.
	TaskID string

	// Use Type to select delay events of a specific type.
	Type string

	// Use Source to select delay events from a specific source.
	Source string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select delay events that occur within the given time range.
	StartTime, EndTime float64

	// Other fields specific to the "delay" table can be added here as needed.
	// For example, if you have additional fields like "what", you can include them here.
}

type ProgressQuery struct {
	// Use ProgressID to select a single progress event by its ID.
	ProgressID string

	// Use TaskID to select all progress events associated with a task.
	TaskID string

	// Use Source to select progress events from a specific source.
	Source string

	// Use Reason to select progress events from a specific source.
	Reason string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select progress events that occur within the given time range.
	StartTime, EndTime float64

	// Other fields specific to the "progress" table can be added here as needed.
	// For example, if you have additional fields like "type", you can include them here.
}


// TraceReader can parse a trace file.
type TraceReader interface {
	// ListComponents returns all the locations used in the trace.
	ListComponents() []string

	// ListTasks queries tasks .
	ListTasks(query TaskQuery) []Task
	ListDelayEvents(query DelayQuery) []DelayEvent
	ListProgressEvents(query ProgressQuery) []ProgressEvent
	ListDependencyEvents() []DependencyEvent
}
