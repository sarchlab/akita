package akita

import (
	"log"
	"reflect"
	"sync"
)

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	HookableBase

	timeLock sync.RWMutex
	time     VTimeInSec
	queue    EventQueue

	simulationEndHandlers []SimulationEndHandler
}

// NewSerialEngine creates a SerialEngine
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)

	e.queue = NewEventQueue()
	//e.queue = NewInsertionQueue()

	return e
}

// Schedule register an event to be happen in the future
func (e *SerialEngine) Schedule(evt Event) {
	e.timeLock.RLock()
	if evt.Time() < e.time {
		e.timeLock.RUnlock()
		log.Panic("scheduling an event earlier than current time")
	}
	e.timeLock.RUnlock()
	e.queue.Push(evt)
}

// Run processes all the events scheduled in the SerialEngine
func (e *SerialEngine) Run() error {
	for {
		if e.queue.Len() == 0 {
			return nil
		}

		evt := e.queue.Pop()
		e.timeLock.RLock()
		if evt.Time() < e.time {
			e.timeLock.RUnlock()
			log.Panicf(
				"cannot run event in the past, evt %s @ %.10f, now %.10f",
				reflect.TypeOf(evt), evt.Time(), e.time,
			)
		}
		e.timeLock.RUnlock()
		e.timeLock.Lock()
		e.time = evt.Time()
		e.timeLock.Unlock()

		e.InvokeHook(evt, e, BeforeEventHookPos, nil)
		handler := evt.Handler()
		handler.Handle(evt)
		e.InvokeHook(evt, e, AfterEventHookPos, nil)
	}
}

// CurrentTime returns the current time at which the engine is at.
// Specifically, the run time of the current event.
func (e *SerialEngine) CurrentTime() VTimeInSec {
	e.timeLock.RLock()
	t := e.time
	e.timeLock.RUnlock()
	return t
}

// RegisterSimulationEndHandler invokes all the registered simulation end
// handler.
func (e *SerialEngine) RegisterSimulationEndHandler(
	handler SimulationEndHandler,
) {
	e.simulationEndHandlers = append(e.simulationEndHandlers, handler)
}

// Finished should be called after the simulation ends. This function
// calls all the registered SimulationEndHandler.
func (e *SerialEngine) Finished() {
	for _, h := range e.simulationEndHandlers {
		h.Handle(e.time)
	}
}
