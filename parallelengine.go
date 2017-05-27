package core

import (
	"log"
	"math"
	"reflect"
	"runtime"
	"sync"
)

// func init() {
// 	runtime.GOMAXPROCS(runtime.NumCPU())
// }

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	paused bool
	// queue   EventQueue
	now     VTimeInSec
	nowLock sync.RWMutex

	eventChan    chan Event
	waitGroup    sync.WaitGroup
	maxGoRoutine int

	queues    []EventQueue
	queueChan chan EventQueue
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.paused = false
	e.eventChan = make(chan Event, 10000)

	e.maxGoRoutine = runtime.NumCPU()
	for i := 0; i < e.maxGoRoutine; i++ {
		e.startWorker()
	}

	numQueues := e.maxGoRoutine
	e.queues = make([]EventQueue, 0, numQueues)
	e.queueChan = make(chan EventQueue, numQueues)
	for i := 0; i < numQueues; i++ {
		queue := NewEventQueue()
		e.queueChan <- queue
		e.queues = append(e.queues, queue)
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
	e.nowLock.RLock()
	if evt.Time() < e.now {
		log.Fatalf("Time inverse, evt %s @ %.10f, now %.10f",
			reflect.TypeOf(evt), evt.Time(), e.now)
	} else if evt.Time() == e.now {
		e.runEvent(evt)
		e.nowLock.RUnlock()
		return
	}
	e.nowLock.RUnlock()

	queue := <-e.queueChan
	queue.Push(evt)
	e.queueChan <- queue
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	defer close(e.eventChan)

	for !e.paused {
		e.emptyQueueChan()

		if !e.hasMoreEvents() {
			return nil
		}

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
	e.nowLock.Lock()
	e.now = triggerTime
	e.nowLock.Unlock()

	for _, queue := range e.queues {
		for queue.Len() > 0 {
			evt := queue.Peek()
			if evt.Time() == triggerTime {
				evt2 := queue.Pop()
				if evt2 != evt {
					log.Fatal("Peek != Pop")
				}
				e.runEvent(evt)
			} else if evt.Time() < triggerTime {
				log.Fatalf("Time inverse, evt %s time %.10f, trigger time %.10f",
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

// func (e *ParallelEngine) canRunEvent(evt Event) bool {
// 	if e.now == 0 || e.now >= evt.Time() {
// 		// for _, h := range e.runningHandler {
// 		// 	if h == evt.Handler() {
// 		// 		return false
// 		// 	}
// 		// }
// 		return true
// 	}
// 	return false
// }

func (e *ParallelEngine) runEvent(evt Event) {
	e.waitGroup.Add(1)

	e.eventChan <- evt
}

// Pause will stop the engine from dispatching more events
func (e *ParallelEngine) Pause() {
	e.paused = true
}
