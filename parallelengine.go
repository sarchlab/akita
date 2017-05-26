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
	paused  bool
	queue   *EventQueue
	now     VTimeInSec
	nowLock sync.RWMutex

	eventChan    chan Event
	waitGroup    sync.WaitGroup
	maxGoRoutine int

	scheduleChan      chan Event
	scheduleWaitGroup sync.WaitGroup
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.queue = NewEventQueue()
	e.eventChan = make(chan Event, 10000)
	e.scheduleChan = make(chan Event, 10000)

	e.maxGoRoutine = runtime.NumCPU()
	for i := 0; i < e.maxGoRoutine; i++ {
		e.startWorker()
	}
	go e.scheduleWorker()

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
	e.nowLock.RLock()
	if evt.Time() <= e.now {
		e.runEvent(evt)
	}
	e.nowLock.RUnlock()

	e.scheduleWaitGroup.Add(1)
	e.scheduleChan <- evt
}

func (e *ParallelEngine) scheduleWorker() {
	for evt := range e.scheduleChan {
		e.queue.Push(evt)
		e.scheduleWaitGroup.Done()
	}
}

func (e *ParallelEngine) popEvent() Event {
	return e.queue.Pop()
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	defer close(e.eventChan)
	defer close(e.scheduleChan)

	e.scheduleWaitGroup.Wait() // In case any event is scheduled before the main loop

	for !e.paused {

		if e.queue.Len() == 0 {
			return nil
		}

		e.runEventsUntilConflict()
		e.waitGroup.Wait()
		e.scheduleWaitGroup.Wait()

		e.nowLock.Lock()
		e.now = 0
		e.nowLock.Unlock()
	}
	return nil
}

func (e *ParallelEngine) runEventsUntilConflict() {
	for e.queue.Len() > 0 {
		evt := e.popEvent()
		if e.canRunEvent(evt) {
			e.nowLock.Lock()
			e.now = evt.Time()
			e.nowLock.Unlock()
			e.runEvent(evt)
		} else {
			e.queue.Push(evt)
			break
		}
	}

}

func (e *ParallelEngine) canRunEvent(evt Event) bool {
	if e.now == 0 || e.now >= evt.Time() {
		return true
	}
	return false
}

func (e *ParallelEngine) runEvent(evt Event) {
	e.waitGroup.Add(1)

	e.eventChan <- evt
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
