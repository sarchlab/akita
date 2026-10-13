package tracing

import "github.com/sarchlab/akita/v5/sim/tasks"

// TaskFilter selects tasks for an observer. A nil filter selects all tasks.
// Time aggregators evaluate it on completion; tag and busy-time observers at start.
type TaskFilter func(tasks.Task) bool
