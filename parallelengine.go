package core

import (
	"container/heap"
	"sync"
)

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	paused    bool
	queueChan chan *EventQueue

	EvtInFlight map[Event]bool
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.EvtInFlight = make(map[Event]bool)

	queue := make(EventQueue, 0, 0)
	heap.Init(&queue)

	e.queueChan = make(chan *EventQueue, 1)
	e.queueChan <- &queue

	return e
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	queue := <-e.queueChan
	heap.Push(queue, evt)
	e.queueChan <- queue
}

func (e *ParallelEngine) runEvent(evt Event, wg *sync.WaitGroup) {

}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	runningHandlers := make(map[Handler]bool)
	var happenTime VTimeInSec

	for !e.paused {
		queue := <-e.queueChan
		if queue.Len() == 0 {
			return nil
		}

		for {
			evt := heap.Pop(queue).(Event)
			time := evt.Time()
			handler := evt.Handler()
			if _, ok := runningHandlers[handler]; ok || time > 0 {
				heap.Push(queue, evt)
				break
			}
			go handler.Handle(evt)
		}

		e.queueChan <- queue

		<-evt.FinishChan()
	}
	return nil
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
