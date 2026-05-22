package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

type EventPrinter struct {
}

func (e *EventPrinter) Handle(event timing.Event) error {
	fmt.Printf("Event: %d\n", event.Time())

	return nil
}

func main() {
	s := simulation.MakeBuilder().Build()

	handler := &EventPrinter{}
	engine := s.GetEngine()

	if registrar, ok := engine.(timing.HandlerRegistrar); ok {
		registrar.RegisterHandler("printer", handler)
	}

	evt := timing.NewEventBase(1, "printer")

	engine.Schedule(evt)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	s.Terminate()
}
