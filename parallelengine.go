package core

import (
	"log"
	"math"
	"reflect"
	"sync"

	"runtime"
)

// func init() {
// 	runtime.GOMAXPROCS(runtime.NumCPU())
// }

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	*HookableBase

	paused bool
	now    VTimeInSec

	eventChan    chan Event
	waitGroup    sync.WaitGroup
	maxGoRoutine int

	queues    []EventQueue
	queueChan chan EventQueue
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)
	e.HookableBase = NewHookableBase()

	e.paused = false
	e.now = -1
	e.eventChan = make(chan Event, 1000)

	e.spawnWorkers()

	numQueues := e.maxGoRoutine * 2
	e.queues = make([]EventQueue, 0, numQueues)
	e.queueChan = make(chan EventQueue, numQueues)
	for i := 0; i < numQueues; i++ {
		queue := NewEventQueue()
		e.queueChan <- queue
		e.queues = append(e.queues, queue)
	}

	return e
}

func (e *ParallelEngine) spawnWorkers() {
	e.maxGoRoutine = runtime.NumCPU() * 2
	for i := 0; i < e.maxGoRoutine; i++ {
		go e.worker()
	}

}

func (e *ParallelEngine) worker() {
	for evt := range e.eventChan {
		e.InvokeHook(evt, e, BeforeEvent, nil)
		handler := evt.Handler()
		handler.Handle(evt)
		e.InvokeHook(evt, e, AfterEvent, nil)
		e.waitGroup.Done()
	}
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	if evt.Time() < e.now {
		log.Panicf("Time inverse, evt %s @ %.10f, now %.10f",
			reflect.TypeOf(evt), evt.Time(), e.now)
	} else if evt.Time() == e.now {
		e.runEvent(evt)
		return
	}

	queue := <-e.queueChan
	queue.Push(evt)
	e.queueChan <- queue
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	for !e.paused {
		if !e.hasMoreEvents() {
			return nil
		}

		e.emptyQueueChan()
		e.runEventsUntilConflict()
		e.waitGroup.Wait()
	}
	return nil
}

func (e *ParallelEngine) emptyQueueChan() {
	for range e.queues {
		<-e.queueChan
	}
}

func (e *ParallelEngine) hasMoreEvents() bool {
	for _, q := range e.queues {
		if q.Len() > 0 {
			return true
		}
	}
	return false
}

func (e *ParallelEngine) runEventsUntilConflict() {

	triggerTime := e.triggerTime()
	e.now = triggerTime

	for _, queue := range e.queues {
		for queue.Len() > 0 {
			evt := queue.Peek()
			if evt.Time() == triggerTime {
				queue.Pop()
				e.runEvent(evt)
			} else if evt.Time() < triggerTime {
				log.Panicf("Time inverse, evt %s time %.10f, trigger time %.10f",
					reflect.TypeOf(evt), evt.Time(), triggerTime)
			} else {
				break
			}
		}
		e.queueChan <- queue
	}
}

func (e *ParallelEngine) triggerTime() VTimeInSec {
	var earliest VTimeInSec

	earliest = math.MaxFloat64
	for _, q := range e.queues {
		if q.Len() == 0 {
			continue
		}

		evt := q.Peek()
		if evt.Time() <= earliest {
			earliest = evt.Time()
		}
	}
	return earliest
}

func (e *ParallelEngine) runEvent(evt Event) {
	e.waitGroup.Add(1)

	e.eventChan <- evt
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}

// CurrentTime returns the current time at which the engine is at. Specifically, the run time of the current event.
func (e *ParallelEngine) CurrentTime() VTimeInSec {
	return e.now
}
