package tracing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type SampleTimeTeller struct {
	time sim.VTimeInSec
}

func (t *SampleTimeTeller) CurrentTime() sim.VTimeInSec {
	return t.time
}

type SampleDomain struct {
	*sim.HookableBase

	timeTeller sim.TimeTeller
	taskIDs    []uint64
	nextID     uint64
}

func (d *SampleDomain) Name() string {
	return "sample domain"
}

func (d *SampleDomain) Start() {
	d.nextID++

	tracing.StartTask(
		d.nextID,
		0,
		d,
		"sampleTaskKind",
		"something",
		nil,
	)

	d.taskIDs = append(d.taskIDs, d.nextID)
}

func (d *SampleDomain) End() {
	tracing.EndTask(
		d.taskIDs[0],
		d,
	)

	d.taskIDs = d.taskIDs[1:]
}

// Example for how to use standard tracers
func ExampleTracer() {
	timeTeller := &SampleTimeTeller{}
	domain := &SampleDomain{
		HookableBase: sim.NewHookableBase(),
		timeTeller:   timeTeller,
	}

	filter := func(t tracing.Task) bool {
		return t.Kind == "sampleTaskKind"
	}

	totalTimeTracer := tracing.NewTotalTimeTracer(timeTeller, filter)
	busyTimeTracer := tracing.NewBusyTimeTracer(timeTeller, filter)
	avgTimeTracer := tracing.NewAverageTimeTracer(timeTeller, filter)
	tracing.CollectTrace(domain, totalTimeTracer)
	tracing.CollectTrace(domain, busyTimeTracer)
	tracing.CollectTrace(domain, avgTimeTracer)

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
