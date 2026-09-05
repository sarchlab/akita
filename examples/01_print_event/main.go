package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

type EventPrinter struct {
}

func (e *EventPrinter) Handle(event timing.Event) {
	fmt.Printf("Event: %d\n", event.Time())

	return
}

func main() {
	s, err := simulation.MakeBuilder().Build()
	if err != nil {
		panic(err)
	}

	handler := &EventPrinter{}
	engine := s.GetEngine()

	if registrar, ok := engine.(timing.HandlerRegistrar); ok {
		registrar.RegisterHandler("printer", handler)
	}

	engine.Schedule(timing.MakeEventBase(timing.IDsFor(engine), 1, "printer"))

	err = engine.Run()
	if err != nil {
		panic(err)
	}

	if err := s.Terminate(); err != nil {
		panic(err)
	}
}
