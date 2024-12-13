package hooking

// A list of hook poses for the hooks to apply to
var (
	HookPosTaskStart = &HookPos{Name: "HookPosTaskStart"}
	HookPosTaskTag   = &HookPos{Name: "HookPosTaskTag"}
	HookPosTaskStep  = &HookPos{Name: "HookPosTaskStep"}
	HookPosTaskEnd   = &HookPos{Name: "HookPosTaskEnd"}
)

// TaskStart is data that is passed to the hook when a task starts.
type TaskStart struct {
	ID       string
	ParentID string
	Kind     string
	What     string
	Where    string
}

// TaskTag is data attached to a task to provide more information about the
// task.
type TaskTag struct {
	TaskID string
	What   string
	Detail string
}

// TaskStep is data that is passed to the hook when a task takes a step.
type TaskStep struct {
	TaskID string
	StepID string
	Kind   string
	What   string
	Detail string
}

// TaskEnd is data that is passed to the hook when a task ends.
type TaskEnd struct {
	ID string
}

type step struct {
	ID     string  `json:"id"`
	Time   float64 `json:"time"`
	Kind   string  `json:"kind"`
	What   string  `json:"what"`
	Detail string  `json:"detail"`
}

type tag struct {
	What   string `json:"what"`
	Detail string `json:"detail"`
}

type task struct {
	ID        string  `json:"id"`
	ParentID  string  `json:"parent_id"`
	Kind      string  `json:"kind"`
	What      string  `json:"what"`
	Where     string  `json:"where"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Steps     []step  `json:"steps"`
	Tags      []tag   `json:"tags"`
}

// TaskFilter is a function that can filter interesting tasks. If this function
// returns true, the task is considered useful.
type TaskFilter func(t TaskStart) bool

// A TimeTeller can tell the current time. This interface is recreated here
// to break a circular dependency between the timing package and the
// hooking package.
type TimeTeller interface {
	Now() float64
}
