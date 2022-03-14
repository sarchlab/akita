package sim

import (
	"log"
	"reflect"
	"sync"
)

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	HookableBase

	timeLock       sync.RWMutex
	time           VTimeInSec
	queue          EventQueue
	secondaryQueue EventQueue

	isPaused     bool
	isPausedLock sync.Mutex
	pauseLock    sync.Mutex

	singleRunLock sync.Mutex

	simulationEndHandlers []SimulationEndHandler
}

// NewSerialEngine creates a SerialEngine
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)

	e.queue = NewEventQueue()
	e.secondaryQueue = NewEventQueue()
	//e.queue = NewInsertionQueue()

	return e
}

// Schedule register an event to be happen in the future
func (e *SerialEngine) Schedule(evt Event) {
	now := e.readNow()
	if evt.Time() < now {
		log.Panic("scheduling an event earlier than current time")
	}

	if evt.IsSecondary() {
		e.secondaryQueue.Push(evt)
		return
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
	e.singleRunLock.Lock()
	defer e.singleRunLock.Unlock()

	for {
		if e.noMoreEvent() {
			return nil
		}

		e.pauseLock.Lock()

		evt := e.nextEvent()
		now := e.readNow()
		if evt.Time() < now {
			log.Panicf(
				"cannot run event in the past, evt %s @ %.10f, now %.10f",
				reflect.TypeOf(evt), evt.Time(), now,
			)
		}
		e.writeNow(evt.Time())

		hookCtx := HookCtx{
			Domain: e,
			Pos:    HookPosBeforeEvent,
			Item:   evt,
		}
		e.InvokeHook(hookCtx)

		handler := evt.Handler()
		_ = handler.Handle(evt)

		hookCtx.Pos = HookPosAfterEvent
		e.InvokeHook(hookCtx)

		e.pauseLock.Unlock()
	}
}

func (e *SerialEngine) noMoreEvent() bool {
	return e.queue.Len() == 0 && e.secondaryQueue.Len() == 0
}

func (e *SerialEngine) nextEvent() Event {
	if e.queue.Len() == 0 {
		return e.secondaryQueue.Pop()
	}

	if e.secondaryQueue.Len() == 0 {
		return e.queue.Pop()
	}

	primaryEvt := e.queue.Peek()
	secondaryEvt := e.secondaryQueue.Peek()

	if primaryEvt.Time() <= secondaryEvt.Time() {
		e.queue.Pop()
		return primaryEvt
	}

	e.secondaryQueue.Pop()
	return secondaryEvt
}

// Pause prevents the SerialEngine to trigger more events.
func (e *SerialEngine) Pause() {
	e.isPausedLock.Lock()
	defer e.isPausedLock.Unlock()

	if e.isPaused {
		return
	}

	e.pauseLock.Lock()
	e.isPaused = true
}

// Continue allows the SerialEngine to trigger more events.
func (e *SerialEngine) Continue() {
	e.isPausedLock.Lock()
	defer e.isPausedLock.Unlock()

	if !e.isPaused {
		return
	}

	e.pauseLock.Unlock()
	e.isPaused = false
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
