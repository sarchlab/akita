package core

import (
	"runtime"
	"sync"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	paused          bool
	queue           *EventQueue
	now             VTimeInSec
	runningHandlers map[Handler]bool
	waitGroup       sync.WaitGroup
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.queue = NewEventQueue()
	e.runningHandlers = make(map[Handler]bool)

	return e
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	// e.queue.Lock()
	// for _, evtInList := range e.queue.events {
	// 	if evtInList == evt {
	// 		debug.PrintStack()
	// 		log.Fatal("Cannot schedule two same event")
	// 	}
	// }
	// e.queue.Unlock()
	e.queue.Push(evt)
}

func (e *ParallelEngine) popEvent() Event {
	return e.queue.Pop()
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	for !e.paused {
		if e.queue.Len() == 0 {
			return nil
		}

		e.runEventsUntilConflict()

		e.waitGroup.Wait()
		e.runningHandlers = make(map[Handler]bool)
		e.now = 0
	}
	return nil
}

func (e *ParallelEngine) runEventsUntilConflict() {
	for e.queue.Len() > 0 {
		evt := e.popEvent()
		if e.canRunEvent(evt) {
			e.runEvent(evt)
		} else {
			e.Schedule(evt)
			break
		}
	}

}

func (e *ParallelEngine) canRunEvent(evt Event) bool {
	if e.now == 0 || e.now >= evt.Time() {
		_, handlerInUse := e.runningHandlers[evt.Handler()]
		if !handlerInUse {
			return true
		}
	}
	return false
}

func (e *ParallelEngine) runEvent(evt Event) {
	e.waitGroup.Add(1)
	e.runningHandlers[evt.Handler()] = true
	e.now = evt.Time()

	go e.runEventGoRoutine(evt)

}

func (e *ParallelEngine) runEventGoRoutine(evt Event) {
	defer e.waitGroup.Done()

	handler := evt.Handler()
	handler.Handle(evt)
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
