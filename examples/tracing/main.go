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

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

// Spec is the worker's configuration.
type Spec struct {
	Freq         timing.Freq `json:"freq"`
	NumJobs      int         `json:"num_jobs"`
	CyclesPerJob int         `json:"cycles_per_job"`
}

// State is the worker's runtime data.
type State struct {
	JobsLeft  int    `json:"jobs_left"`
	Working   bool   `json:"working"`
	CountDown int    `json:"count_down"`
	CurTaskID uint64 `json:"cur_task_id"`
	NextID    uint64 `json:"next_id"`
}

// Ports is empty: the worker talks to no one.
type Ports struct{}

// Middlewares holds the worker's behavior.
type Middlewares struct {
	// Work runs one cycle of the current job, or starts the next one.
	Work *workerMW
}

// Comp is the worker, a ticking component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the worker.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz, NumJobs: 3, CyclesPerJob: 4},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

// newState gives the worker all of its jobs up front.
func newState(c *Comp) State {
	return State{JobsLeft: c.Spec.NumJobs}
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Work: &workerMW{comp: c}}
}

type workerMW struct {
	comp *Comp
}

func (m *workerMW) Handle(_ timing.Event) bool {
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
		s.CountDown = m.comp.Spec.CyclesPerJob
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
	sim := modeling.NewStandaloneSimulation(engine)

	worker := Definition.Builder().
		WithSimulation(sim).
		Build("Worker")
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	// A tracer only cares about tasks whose Kind matches this filter.
	onlyJobs := func(t tracing.TaskStart) bool { return t.Kind == "job" }

	busy := tracing.NewBusyTimeTracer(onlyJobs)
	avg := tracing.NewAverageTimeTracer(onlyJobs)

	tracing.CollectTrace(worker, busy)
	tracing.CollectTrace(worker, avg)

	worker.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("jobs traced:  %d\n", avg.TotalCount())
	fmt.Printf("busy time:    %d ps\n", busy.BusyTime())
	fmt.Printf("average time: %d ps\n", avg.AverageTime())
}
