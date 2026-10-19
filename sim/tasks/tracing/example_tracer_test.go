package tracing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/internal/idgen"

	"github.com/sarchlab/akita/v5/sim/naming"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type SampleTimeTeller struct {
	time timing.VTimeInPicoSec
}

func (t *SampleTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return t.time
}

type SampleDomain struct {
	tracker *tasks.Tracker

	taskIDs []uint64
}

func (d *SampleDomain) Name() string {
	return "sample domain"
}

func (d *SampleDomain) Start() {
	id := d.TaskTracker().StartTask(d, tasks.StartInfo{
		Kind: "sampleTaskKind",
		What: "something",
	})

	d.taskIDs = append(d.taskIDs, id)
}

func (d *SampleDomain) End() {
	d.TaskTracker().EndTask(
		d.taskIDs[0],
	)

	d.taskIDs = d.taskIDs[1:]
}

// Example for how to use standard tracers
func ExampleNewTotalTimeTracer() {
	timeTeller := &SampleTimeTeller{}
	domain := &SampleDomain{}

	ids := &idgen.IDGenerator{}
	domain.tracker = tasks.NewTracker(timeTeller, ids.NewID, func(n naming.Named) bool { return n == domain })
	filter := func(t tasks.Task) bool {
		return t.Kind == "sampleTaskKind"
	}

	totalTimeTracer := tracing.NewTotalTimeTracer(filter)
	busyTimeTracer := tracing.NewBusyTimeTracer(filter)
	avgTimeTracer := tracing.NewAverageTimeTracer(filter)
	domain.TaskTracker().Subscribe(totalTimeTracer, domain)
	domain.TaskTracker().Subscribe(busyTimeTracer, domain)
	domain.TaskTracker().Subscribe(avgTimeTracer, domain)

	timeTeller.time = 10

	domain.Start()

	timeTeller.time = 15

	domain.Start()

	timeTeller.time = 20

	domain.End()

	timeTeller.time = 30

	domain.End()

	fmt.Println(totalTimeTracer.TotalTime())
	fmt.Println(busyTimeTracer.BusyTime())
	fmt.Println(avgTimeTracer.AverageTime())

	// Output:
	// 25
	// 20
	// 12
}

func (d *SampleDomain) TaskTracker() *tasks.Tracker {
	return d.tracker
}
