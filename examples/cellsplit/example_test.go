package cellsplit

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/sim"
)

var endTime = sim.VTimeInSec(10)
var engine sim.Engine
var randGen *rand.Rand

// splitEvent is an event that splits a cell into two cells.
type splitEvent struct {
	time    sim.VTimeInSec
	handler sim.Handler
}

func (e splitEvent) Time() sim.VTimeInSec {
	return e.time
}

func (e splitEvent) Handler() sim.Handler {
	return e.handler
}

func (e splitEvent) IsSecondary() bool {
	return false
}

type handler struct {
	count int
}

func (h *handler) Handle(e sim.Event) error {
	h.count += 1

	// fmt.Printf("Cell count: %d\n", h.count)

	h.scheduleNextSplitEvent(e.Time())
	h.scheduleNextSplitEvent(e.Time())

	return nil
}

func (h *handler) scheduleNextSplitEvent(now sim.VTimeInSec) {
	timeToSplitLeft := sim.VTimeInSec(randGen.Float64() + 1)
	nextEvt := splitEvent{
		time:    now + timeToSplitLeft,
		handler: h,
	}

	if nextEvt.time < endTime {
		engine.Schedule(nextEvt)
	}
}

func Example_cellSplit() {
	randGen = rand.New(rand.NewSource(0))

	engine = sim.NewSerialEngine()
	h := handler{
		count: 1,
	}

	firstEvtTime := sim.VTimeInSec(randGen.Float64() + 1)
	firstEvt := splitEvent{
		time:    firstEvtTime,
		handler: &h,
	}

	engine.Schedule(firstEvt)

	engine.Run()

	fmt.Printf("Cell count at time %.0f: %d\n", endTime, h.count)

	// Output:
	// Cell count at time 10: 75
}
