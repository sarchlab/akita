// Command customtracer shows how to write your own tracer.
//
// A tracer is any type that implements the tracing.Tracer interface. Here a
// hand-written maxDurationTracer watches the worker's job tasks and reports
// the longest one. It is attached exactly like a built-in tracer, with
// tracing.CollectTrace.
package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type workerSpec struct {
	NumJobs      int `json:"num_jobs"`
	CyclesPerJob int `json:"cycles_per_job"`
}

type workerState struct {
	JobsLeft  int    `json:"jobs_left"`
	Working   bool   `json:"working"`
	CountDown int    `json:"count_down"`
	CurTaskID uint64 `json:"cur_task_id"`
	NextID    uint64 `json:"next_id"`
}

// Comp is the worker component.
type Comp = modeling.Component[workerSpec, workerState, modeling.None]

type workerMW struct {
	comp *Comp
}

func (m *workerMW) Tick() bool {
	s := &m.comp.State

	if !s.Working {
		if s.JobsLeft == 0 {
			return false
		}

		s.NextID++
		s.CurTaskID = s.NextID
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:   s.CurTaskID,
			Kind: "job",
			What: fmt.Sprintf("job-%d", s.CurTaskID),
		})

		s.Working = true
		// Jobs get progressively longer so "the longest" is meaningful.
		s.CountDown = m.comp.Spec().CyclesPerJob * int(s.CurTaskID)
		s.JobsLeft--

		return true
	}

	s.CountDown--
	if s.CountDown == 0 {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: s.CurTaskID})
		s.Working = false
	}

	return true
}

// maxDurationTracer is a custom tracer. Each trace event carries the time it
// happened, so the tracer just records every task's start time and tracks the
// longest start-to-end span. It embeds tracing.NopTracer, so it only has to
// implement the two methods it actually cares about.
type maxDurationTracer struct {
	tracing.NopTracer
	starts map[uint64]timing.VTimeInPicoSec
	max    timing.VTimeInPicoSec
}

func newMaxDurationTracer() *maxDurationTracer {
	return &maxDurationTracer{starts: make(map[uint64]timing.VTimeInPicoSec)}
}

func (t *maxDurationTracer) StartTask(task tracing.TaskStart) {
	t.starts[task.ID] = task.Time
}

func (t *maxDurationTracer) EndTask(task tracing.TaskEnd) {
	start, ok := t.starts[task.ID]
	if !ok {
		return
	}
	delete(t.starts, task.ID)

	if d := task.Time - start; d > t.max {
		t.max = d
	}
}

func (t *maxDurationTracer) MaxDuration() timing.VTimeInPicoSec { return t.max }

func main() {
	engine := timing.NewSerialEngine()
	registrar := modeling.NewStandaloneRegistrar(engine)

	worker := modeling.NewBuilder[workerSpec, workerState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(workerSpec{NumJobs: 3, CyclesPerJob: 4}).
		Build("Worker")
	worker.AddMiddleware(&workerMW{comp: worker})
	registrar.RegisterComponent(worker)

	tracer := newMaxDurationTracer()
	tracing.CollectTrace(worker, tracer)

	worker.State.JobsLeft = worker.Spec().NumJobs
	worker.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("longest job: %d ps\n", tracer.MaxDuration())
}
