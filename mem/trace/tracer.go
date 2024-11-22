// Package trace provides a tracer that can trace memory system tasks.
package trace

import (
	"log"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

// A tracer is a hook that can record the actions of a memory model into
// traces.
type tracer struct {
	timeTeller sim.TimeTeller
	logger     *log.Logger
}

// StartTask marks the start of a memory transaction
func (t *tracer) StartTask(task tracing.Task) {
	task.StartTime = t.timeTeller.CurrentTime()

	req, ok := task.Detail.(mem.AccessReq)
	if !ok {
		return
	}
	t.logger.Printf("start, %.12f, %s, %s, %s, 0x%x, %d\n",
		task.StartTime, task.Where, task.ID, task.What,
		req.GetAddress(), req.GetByteSize())
}

// StepTask marks the memory transaction has completed a milestone
func (t *tracer) StepTask(task tracing.Task) {
	task.Steps[0].Time = t.timeTeller.CurrentTime()

	t.logger.Printf("step, %.12f, %s, %s\n",
		task.Steps[0].Time,
		task.ID,
		task.Steps[0].What)
}

// EndTask marks the end of a memory transaction
func (t *tracer) EndTask(task tracing.Task) {
	task.EndTime = t.timeTeller.CurrentTime()

	t.logger.Printf("end, %.12f, %s\n", task.EndTime, task.ID)
}


// DelayTask marks the delay of a memory transaction
func (t *tracer) DelayTask(delayEvent tracing.DelayEvent) {
	delayEvent.Time = t.timeTeller.CurrentTime()
}

// ProgressTask marks the progress of a memory transaction
func (t *tracer) ProgressTask(progressEvent tracing.ProgressEvent) {
	progressEvent.Time = t.timeTeller.CurrentTime()
}

// DependencyTask marks the dependency of a memory transaction buffee
func (t *tracer) DependencyTask(event tracing.DependencyEvent) {
	// event.Time = t.timeTeller.CurrentTime()
}
// NewTracer creates a new Tracer.
func NewTracer(logger *log.Logger, timeTeller sim.TimeTeller) tracing.Tracer {
	t := new(tracer)
	t.logger = logger
	t.timeTeller = timeTeller
	return t
}
