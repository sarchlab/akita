package akita

import (
	"log"
	"reflect"
)

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	*HookableBase

	time  VTimeInSec
	queue EventQueue
}

func (e *SerialEngine) RegisterPostSimulationHandler(handler Handler) {
	panic("implement me")
}

// NewSerialEngine creates a SerialEngine
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)
	e.HookableBase = NewHookableBase()

	e.queue = NewEventQueue()
	//e.queue = NewInsertionQueue()

	return e
}

// Schedule register an event to be happen in the future
func (e *SerialEngine) Schedule(evt Event) {
	if evt.Time() < e.time {
		log.Panic("scheduling an event earlier than current time")
	}
	e.queue.Push(evt)
	//fmt.Printf("Schedule event %.10f, %s\n", evt.Time(), reflect.TypeOf(evt))
}

// Run processes all the events scheduled in the SerialEngine
func (e *SerialEngine) Run() error {
	for {
		if e.queue.Len() == 0 {
			return nil
		}

		evt := e.queue.Pop()
		e.InvokeHook(evt, e, BeforeEventHookPos, nil)
		if evt.Time() < e.time {
			log.Panicf("cannot run event in the past, evt %s @ %.10f, now %.10f",
				reflect.TypeOf(evt), evt.Time(), e.time)
		}
		e.time = evt.Time()

		handler := evt.Handler()
		handler.Handle(evt)
		e.InvokeHook(evt, e, AfterEventHookPos, nil)
	}

	return nil
}

// CurrentTime returns the current time at which the engine is at.
// Specifically, the run time of the current event.
func (e *SerialEngine) CurrentTime() VTimeInSec {
	return e.time
}
