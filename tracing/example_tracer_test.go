package tracing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/hooking"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type SampleTimeTeller struct {
	time timing.VTimeInPicoSec
}

func (t *SampleTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return t.time
}

type SampleDomain struct {
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

	tracing.StartTask(d, tracing.TaskStart{
		ID:   d.nextID,
		Kind: "sampleTaskKind",
		What: "something",
	})

	d.taskIDs = append(d.taskIDs, d.nextID)
}

func (d *SampleDomain) End() {
	tracing.EndTask(d, tracing.TaskEnd{
		ID: d.taskIDs[0],
	})

	d.taskIDs = d.taskIDs[1:]
}

// Example for how to use standard tracers
func ExampleTracer() {
	timeTeller := &SampleTimeTeller{}
	domain := &SampleDomain{
		HookableBase: hooking.NewHookableBase(),
		timeTeller:   timeTeller,
	}

	filter := func(t tracing.TaskStart) bool {
		return t.Kind == "sampleTaskKind"
	}

	totalTimeTracer := tracing.NewTotalTimeTracer(filter)
	busyTimeTracer := tracing.NewBusyTimeTracer(filter)
	avgTimeTracer := tracing.NewAverageTimeTracer(filter)
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
