package tracing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type SampleTimeTeller struct {
	time timing.VTimeInPicoSec
}

func (t *SampleTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return t.time
}

type SampleDomain struct {
	sim timing.Simulation
	*hooking.HookableBase

	timeTeller timing.TimeTeller
	taskIDs    []uint64
	nextID     uint64
}

func (d *SampleDomain) Name() string {
	return "sample domain"
}

func (d *SampleDomain) CurrentTime() timing.VTimeInPicoSec {
	return d.timeTeller.CurrentTime()
}

func (d *SampleDomain) Start() {
	d.nextID++

	id := tasks.Start(d, tasks.StartInfo{
		Kind: "sampleTaskKind",
		What: "something",
	})

	d.taskIDs = append(d.taskIDs, id)
}

func (d *SampleDomain) End() {
	tasks.End(d, tasks.EndInfo{
		ID: d.taskIDs[0],
	})

	d.taskIDs = d.taskIDs[1:]
}

// Example for how to use standard tracers
func ExampleNewTotalTimeTracer() {
	timeTeller := &SampleTimeTeller{}
	domain := &SampleDomain{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine()).WithTaskTracking(),
		HookableBase: &hooking.HookableBase{},
		timeTeller:   timeTeller,
	}

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

func (c *SampleDomain) NewID() uint64 { return c.sim.NewID() }

func (d *SampleDomain) TaskTracker() *tasks.Tracker {
	return d.sim.(interface{ Tasks() *tasks.Tracker }).Tasks()
}
