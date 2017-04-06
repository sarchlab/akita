package core

import "sync"

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	paused          bool
	queue           *EventQueue
	evtChan         chan Event
	now             VTimeInSec
	runningHandlers map[Handler]bool
	waitGroup       sync.WaitGroup

	MaxGoRoutine int
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.MaxGoRoutine = 4
	e.queue = NewEventQueue()
	e.runningHandlers = make(map[Handler]bool)
	e.evtChan = make(chan Event, e.MaxGoRoutine*10)

	for i := 0; i < e.MaxGoRoutine; i++ {
		go e.startEventWorker()
	}

	return e
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	e.queue.Push(evt)
}

func (e *ParallelEngine) popEvent() Event {
	return e.queue.Pop()
}

func (e *ParallelEngine) startEventWorker() {
	for {
		evt, running := <-e.evtChan
		if !running {
			return
		}
		handler := evt.Handler()
		handler.Handle(evt)

		e.eventComplete(evt)
	}
}

func (e *ParallelEngine) eventComplete(evt Event) {
	e.waitGroup.Done()
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	defer close(e.evtChan)
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
		evt := e.queue.Peek()
		if e.canRunEvent(evt) {
			e.popEvent()
			e.runEvent(evt)
		} else {
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
	// e.evtInFlightMutex.Lock()
	e.runningHandlers[evt.Handler()] = true
	e.now = evt.Time()
	// e.evtInFlightMutex.Unlock()

	e.evtChan <- evt
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
