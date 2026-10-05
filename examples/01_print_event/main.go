package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type EventPrinter struct {
}

func (e *EventPrinter) Handle(event timing.Event) {
	fmt.Printf("Event: %d\n", event.Time())
}

func main() {
	s := sim.MakeBuilder().Build()

	handler := &EventPrinter{}
	engine := s.Engine()

	engine.RegisterHandler("printer", handler)

	engine.Schedule(timing.MakeEventBase(s.NewID(), 1, "printer"))

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	s.Terminate()
}
