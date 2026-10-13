// Command customtracer shows how to write your own tracer.
//
// A tracer is any type that implements the tasks.Observer interface. Here a
// hand-written maxDurationTracer watches the worker's job tasks and reports
// the longest one. It is attached exactly like a built-in tracer, with
// Tracker.Subscribe.
package main

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
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

		s.NextID++
		s.CurTaskID = tasks.Start(m.comp, tasks.StartInfo{
			Kind: "job",
			What: fmt.Sprintf("job-%d", s.NextID),
		})

		s.Working = true
		// Jobs get progressively longer so "the longest" is meaningful.
		s.CountDown = m.comp.Spec.CyclesPerJob * int(s.NextID)
		s.JobsLeft--

		return true
	}

	s.CountDown--
	if s.CountDown == 0 {
		tasks.End(m.comp, tasks.EndInfo{ID: s.CurTaskID})
		s.Working = false
	}

	return true
}

// maxDurationTracer uses complete end records to track the longest job. It
// embeds NopObserver because it needs no start, tag, or milestone callback.
type maxDurationTracer struct {
	tasks.NopObserver
	lock sync.Mutex
	max  timing.VTimeInPicoSec
}

func newMaxDurationTracer() *maxDurationTracer { return &maxDurationTracer{} }

func (t *maxDurationTracer) TaskEnded(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if d := task.EndTime - task.StartTime; d > t.max {
		t.max = d
	}
}

func (t *maxDurationTracer) MaxDuration() timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.max
}

func main() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine).WithTaskTracking()

	worker := Definition.Builder().
		WithSimulation(sim).
		Build("Worker")
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	tracer := newMaxDurationTracer()
	worker.TaskTracker().Subscribe(tracer, worker)

	worker.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("longest job: %d ps\n", tracer.MaxDuration())
}
