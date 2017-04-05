package core

import (
	"container/heap"
	"log"
	"sync"
)

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	paused          bool
	queue           *EventQueue
	evtChan         chan Event
	nextRound       chan bool
	now             VTimeInSec
	runningHandlers map[Handler]bool

	evtCompleteChan  chan bool
	MaxGoRoutine     int
	evtInFlightMutex sync.Mutex
	EvtInFlight      map[Event]bool
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.EvtInFlight = make(map[Event]bool)
	e.MaxGoRoutine = 60
	e.queue = NewEventQueue()
	e.evtChan = make(chan Event)
	e.runningHandlers = make(map[Handler]bool)
	e.nextRound = make(chan bool)
	e.evtCompleteChan = make(chan bool)

	for i := 0; i < e.MaxGoRoutine; i++ {
		go e.startEventWorker()
	}

	return e
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	heap.Push(e.queue, evt)
}

func (e *ParallelEngine) popEvent() Event {
	return heap.Pop(e.queue).(Event)
}

func (e *ParallelEngine) startEventWorker() {
	for {
		evt, running := <-e.evtChan
		if !running {
			return
		}
		handler := evt.Handler()
		handler.Handle(evt)
		// <-evt.FinishChan()

		e.eventComplete(evt)
	}
}

func (e *ParallelEngine) eventComplete(evt Event) {
	log.Printf("Complete event %+v\n", evt)

	e.evtInFlightMutex.Lock()
	delete(e.EvtInFlight, evt)
	delete(e.runningHandlers, evt.Handler())
	e.evtInFlightMutex.Unlock()

	log.Printf("Complete event %+v\n", evt)
	e.evtCompleteChan <- true
	log.Printf("Complete event %+v\n", evt)
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	defer close(e.evtChan)
	for !e.paused {
		log.Printf("QueueLength before round start %d", e.queue.Len())
		log.Printf("RoundStart\n")
		if e.queue.Len() == 0 {
			return nil
		}

		e.runEventsUntilConflict()

		log.Printf("QueueLength after run %d", e.queue.Len())
		// Wait for all the event to complete
		for {
			<-e.evtCompleteChan
			e.evtInFlightMutex.Lock()
			if len(e.EvtInFlight) == 0 {
				e.now = 0
				e.evtInFlightMutex.Unlock()
				break
			}
			e.evtInFlightMutex.Unlock()
		}
		log.Printf("NextRound\n")

	}
	return nil
}

func (e *ParallelEngine) runEventsUntilConflict() {
	for e.queue.Len() > 0 {
		evt := e.popEvent()
		log.Printf("QueueLength %d", e.queue.Len())
		if e.canRunEvent(evt) {
			e.runEvent(evt)
		} else {
			e.Schedule(evt)
			break
		}
	}

}

func (e *ParallelEngine) canRunEvent(evt Event) bool {
	e.evtInFlightMutex.Lock()
	defer e.evtInFlightMutex.Unlock()
	if e.now == 0 || e.now >= evt.Time() {
		_, handlerInUse := e.runningHandlers[evt.Handler()]
		if !handlerInUse {
			log.Printf("Can run event %+v", evt)
			return true
		}
		log.Printf("Cannot run event, handler in use %+v", evt)
	}
	log.Printf("Cannot run event, time advance %+v", evt)
	return false
}

func (e *ParallelEngine) runEvent(evt Event) {
	log.Printf("Run event %+v\n", evt)

	e.evtInFlightMutex.Lock()
	e.EvtInFlight[evt] = true
	e.runningHandlers[evt.Handler()] = true
	e.now = evt.Time()
	e.evtInFlightMutex.Unlock()

	e.evtChan <- evt
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
