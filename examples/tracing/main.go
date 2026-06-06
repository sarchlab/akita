// Command tracing shows how to measure work with tracing tasks.
//
// A single ticking "worker" component processes a fixed number of jobs, each
// taking a fixed number of cycles. The worker wraps every job in a tracing
// task (StartTask / EndTask). Two built-in tracers — a BusyTimeTracer and an
// AverageTimeTracer — are attached from the outside and report how long the
// worker was busy and how long an average job took, in picoseconds.
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

		// Start a new job and open a tracing task for it.
		s.NextID++
		s.CurTaskID = s.NextID
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:   s.CurTaskID,
			Kind: "job",
			What: fmt.Sprintf("job-%d", s.CurTaskID),
		})

		s.Working = true
		s.CountDown = m.comp.Spec().CyclesPerJob
		s.JobsLeft--

		return true
	}

	// Working: count down, and close the task when the job finishes.
	s.CountDown--
	if s.CountDown == 0 {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: s.CurTaskID})
		s.Working = false
	}

	return true
}

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

	// A tracer only cares about tasks whose Kind matches this filter.
	onlyJobs := func(t tracing.TaskStart) bool { return t.Kind == "job" }

	busy := tracing.NewBusyTimeTracer(onlyJobs)
	avg := tracing.NewAverageTimeTracer(onlyJobs)

	tracing.CollectTrace(worker, busy)
	tracing.CollectTrace(worker, avg)

	worker.State.JobsLeft = worker.Spec().NumJobs
	worker.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("jobs traced:  %d\n", avg.TotalCount())
	fmt.Printf("busy time:    %d ps\n", busy.BusyTime())
	fmt.Printf("average time: %d ps\n", avg.AverageTime())
}
