package core

import (
	"runtime"
	"sync"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	// log.Printf("GOMAXPROCS is set to %d", runtime.NumCPU())
}

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	paused          bool
	queue           *EventQueue
	now             VTimeInSec
	runningHandlers map[Handler]bool
	waitGroup       sync.WaitGroup

	eventChan    chan Event
	maxGoRoutine int
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.queue = NewEventQueue()
	e.runningHandlers = make(map[Handler]bool)

	e.maxGoRoutine = runtime.NumCPU() - 1
	// log.Printf("Using parallel enging with worker number %d\n", e.maxGoRoutine) e.eventChan = make(chan Event, 10000)
	for i := 0; i < e.maxGoRoutine; i++ {
		e.startWorker()
	}

	return e
}

func (e *ParallelEngine) startWorker() {
	go e.worker()
}

func (e *ParallelEngine) worker() {
	for evt := range e.eventChan {
		handler := evt.Handler()
		handler.Handle(evt)
		e.waitGroup.Done()
	}
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	e.queue.Push(evt)
}

func (e *ParallelEngine) popEvent() Event {
	return e.queue.Pop()
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	defer close(e.eventChan)

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
	// runWidth := 0
	for e.queue.Len() > 0 {
		evt := e.popEvent()
		if e.canRunEvent(evt) {
			e.runEvent(evt)
			// runWidth++
			// log.Printf("Lauching %s to %s\n", reflect.TypeOf(evt), reflect.TypeOf(evt.Handler()))
		} else {
			// log.Printf("Event Run width : %d\n", runWidth)
			e.Schedule(evt)
			break
		}
	}

}

func (e *ParallelEngine) canRunEvent(evt Event) bool {
	if e.now == 0 || e.now >= evt.Time() {
		return true
		// _, handlerInUse := e.runningHandlers[evt.Handler()]
		// if !handlerInUse {
		// 	return true
		// }
	}
	return false
}

func (e *ParallelEngine) runEvent(evt Event) {
	e.waitGroup.Add(1)
	e.runningHandlers[evt.Handler()] = true
	e.now = evt.Time()

	e.eventChan <- evt
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
