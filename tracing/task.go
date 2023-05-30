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

// TaskFilter is a function that can filter interesting tasks. If this function
// returns true, the task is considered useful.
type TaskFilter func(t Task) bool
