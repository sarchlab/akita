package akita

import (
	"log"
	"reflect"
	"sync"
)

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	HookableBase

	timeLock  sync.RWMutex
	time      VTimeInSec
	queue     EventQueue
	pauseLock sync.Mutex

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
	now := e.readNow()
	if evt.Time() < now {
		log.Panic("scheduling an event earlier than current time")
	}
	e.queue.Push(evt)
}

func (e *SerialEngine) readNow() VTimeInSec {
	e.timeLock.RLock()
	t := e.time
	e.timeLock.RUnlock()
	return t
}

func (e *SerialEngine) writeNow(t VTimeInSec) {
	e.timeLock.Lock()
	e.time = t
	e.timeLock.Unlock()
}

// Run processes all the events scheduled in the SerialEngine
func (e *SerialEngine) Run() error {
	for {
		if e.queue.Len() == 0 {
			return nil
		}

		e.pauseLock.Lock()

		evt := e.queue.Pop()
		now := e.readNow()
		if evt.Time() < now {
			log.Panicf(
				"cannot run event in the past, evt %s @ %.10f, now %.10f",
				reflect.TypeOf(evt), evt.Time(), now,
			)
		}
		e.writeNow(evt.Time())
		now = evt.Time()

		hookCtx := HookCtx{
			Domain: e,
			Now:    now,
			Pos:    HookPosBeforeEvent,
			Item:   evt,
		}
		e.InvokeHook(&hookCtx)

		handler := evt.Handler()
		handler.Handle(evt)

		hookCtx.Pos = HookPosAfterEvent
		e.InvokeHook(&hookCtx)

		e.pauseLock.Unlock()
	}
}

// Pause pervents the SerialEngine to trigger more events.
func (e *SerialEngine) Pause() {
	e.pauseLock.Lock()
}

// Continue allows the SerialEngine to trigger more events.
func (e *SerialEngine) Continue() {
	e.pauseLock.Unlock()
}

// CurrentTime returns the current time at which the engine is at.
// Specifically, the run time of the current event.
func (e *SerialEngine) CurrentTime() VTimeInSec {
	return e.readNow()
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
	now := e.readNow()
	for _, h := range e.simulationEndHandlers {
		h.Handle(now)
	}
}
