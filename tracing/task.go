package tracing

import "github.com/sarchlab/akita/v3/sim"

// A TaskStep represents a milestone in the processing of task
type TaskStep struct {
	Time sim.VTimeInSec `json:"time"`
	What string         `json:"what"`
}

// A Task is a task
type Task struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id"`
	Kind       string         `json:"kind"`
	What       string         `json:"what"`
	Where      string         `json:"where"`
	StartTime  sim.VTimeInSec `json:"start_time"`
	EndTime    sim.VTimeInSec `json:"end_time"`
	Steps      []TaskStep     `json:"steps"`
	Detail     interface{}    `json:"-"`
	ParentTask *Task          `json:"-"`
}


// DelayEvent represents a delay event related to a task.
type DelayEvent struct {
	EventID string         `json:"event_id"`
	TaskID  string         `json:"task_id"`
	Type    string         `json:"type"`
	What    string         `json:"what"`
	Source  string         `json:"source"`
	Time    sim.VTimeInSec `json:"time"`
}

// ProgressEvent represents a progress event related to a task.
type ProgressEvent struct {
	ProgressID string         `json:"progress_id"`
	TaskID     string         `json:"task_id"`
	Source     string         `json:"source"`
	Time       sim.VTimeInSec `json:"time"`
	Reason     string         `json:"reason"`
}



// DependencyEvent represents a dependency event related to a step task.
type DependencyEvent struct {
	ProgressID  string         `json:"progress_id"`
	DependentID []string   `json:"dependent_id"`
	DependentIDJSON string
}
// TaskFilter is a function that can filter interesting tasks. If this function
// returns true, the task is considered useful.
type TaskFilter func(t Task) bool
