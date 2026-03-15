package main

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
)

var endTime = sim.VTimeInSec(10_000_000_000_000) // 10 seconds in picoseconds
var engine sim.Engine
var randGen *rand.Rand

// splitEvent is an event that splits a cell into two cells.
type splitEvent struct {
	time    sim.VTimeInSec
	handler sim.Handler
	id      int
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

	evt := e.(splitEvent)
	fmt.Printf("Cell %d split at %d ps, current count: %d\n",
		evt.id, evt.Time(), h.count)

	h.scheduleNextSplitEvent(evt.Time(), evt.id)
	h.scheduleNextSplitEvent(evt.Time(), h.count) // h.count is the new cell

	return nil
}

func (h *handler) scheduleNextSplitEvent(now sim.VTimeInSec, id int) {
	timeUntilNextSplit := sim.VTimeInSec(uint64((randGen.Float64() + 1) * 1e12))
	nextEvt := splitEvent{
		time:    now + timeUntilNextSplit,
		handler: h,
		id:      id,
	}

	if nextEvt.time < endTime {
		engine.Schedule(nextEvt)
	}
}

func main() {
	randGen = rand.New(rand.NewSource(0))

	s := simulation.MakeBuilder().Build()
	engine = s.GetEngine()
	h := handler{
		count: 1,
	}

	firstEvtTime := sim.VTimeInSec(uint64((randGen.Float64() + 1) * 1e12))
	firstEvt := splitEvent{
		time:    firstEvtTime,
		handler: &h,
		id:      0,
	}

	engine.Schedule(firstEvt)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	s.Terminate()

	fmt.Printf("Cell count at time %d ps: %d\n", endTime, h.count)
}
