package tracing

// A Tracer can collect task traces
type Tracer interface {
	StartTask(task Task)
	StepTask(task Task)
	EndTask(task Task)
	DelayTask(delayEvent DelayEvent)
	ProgressTask(progressEvent ProgressEvent)
	DependencyTask(event DependencyEvent)
}
