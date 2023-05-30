package tracing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
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
	taskIDs    []int
	nextID     int
}

func (d *SampleDomain) Name() string {
	return "sample domain"
}

func (d *SampleDomain) Start() {
	tracing.StartTask(
		fmt.Sprintf("%d", d.nextID),
		"",
		d,
		"sampleTaskKind",
		"something",
		nil,
	)
	d.taskIDs = append(d.taskIDs, d.nextID)

	d.nextID++
}

func (d *SampleDomain) End() {
	tracing.EndTask(
		fmt.Sprintf("%d", d.taskIDs[0]),
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

	timeTeller.time = 1
	domain.Start()
	timeTeller.time = 1.5
	domain.Start()
	timeTeller.time = 2
	domain.End()
	timeTeller.time = 3
	domain.End()

	fmt.Println(totalTimeTracer.TotalTime())
	fmt.Println(busyTimeTracer.BusyTime())
	fmt.Println(avgTimeTracer.AverageTime())

	// Output:
	// 2.5
	// 2
	// 1.25
}
