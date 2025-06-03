package main

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
)

type EventPrinter struct {
}

func (e *EventPrinter) Handle(event sim.Event) error {
	fmt.Printf("Event: %.10f\n", event.Time())

	return nil
}

func main() {
	s := simulation.MakeBuilder().Build()

	handler := &EventPrinter{}
	evt := sim.NewEventBase(1, handler)

	engine := s.GetEngine()
	engine.Schedule(evt)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	s.Terminate()
}
