package akita

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

	postSimHandlers []Handler
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)
	e.HookableBase = NewHookableBase()

	e.paused = false
	e.now = 0
	e.eventChan = make(chan Event, 10000)

	e.maxGoRoutine = runtime.GOMAXPROCS(0)
	numQueues := runtime.GOMAXPROCS(0)

	e.spawnWorkers()
	e.queues = make([]EventQueue, 0, numQueues)
	e.queueChan = make(chan EventQueue, numQueues)
	for i := 0; i < numQueues; i++ {
		queue := NewEventQueue()
		//queue := NewInsertionQueue()
		e.queueChan <- queue
		e.queues = append(e.queues, queue)
	}

	return e
}

func (e *ParallelEngine) spawnWorkers() {
	for i := 0; i < e.maxGoRoutine; i++ {
		go e.worker()
	}

}

func (e *ParallelEngine) worker() {
	for evt := range e.eventChan {
		e.InvokeHook(evt, e, BeforeEventHookPos, nil)
		handler := evt.Handler()
		handler.Handle(evt)
		e.InvokeHook(evt, e, AfterEventHookPos, nil)
		e.waitGroup.Done()
	}
}

func (e *ParallelEngine) RegisterPostSimulationHandler(handler Handler) {
	e.postSimHandlers = append(e.postSimHandlers, handler)
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	//fmt.Printf("Schedule event %.10f, %s\n", evt.Time(), reflect.TypeOf(evt))
	if evt.Time() < e.now {
		log.Panicf("cannot schedule event in the past, evt %s @ %.10f, now %.10f",
			reflect.TypeOf(evt), evt.Time(), e.now)
	}

	if evt.Time() == e.now && e.now != 0 {
		e.runEventWithTempWorker(evt)
		return
	}

	queue := <-e.queueChan
	queue.Push(evt)
	e.queueChan <- queue
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	for {
		if !e.hasMoreEvents() {
			e.triggerPostSimulationHandlers()
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
				//e.runEvent(evt)
				e.runEventWithTempWorker(evt)
			} else if evt.Time() < triggerTime {
				log.Panicf("cannot run event in the past, evt %s @ %.10f, now %.10f",
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
		if evt.Time() < earliest {
			earliest = evt.Time()
		}
	}
	return earliest
}

func (e *ParallelEngine) runEvent(evt Event) {
	e.waitGroup.Add(1)

	e.eventChan <- evt
}

func (e *ParallelEngine) runEventWithTempWorker(evt Event) {
	e.waitGroup.Add(1)
	go e.tempWorkerRun(evt)
}

func (e *ParallelEngine) tempWorkerRun(evt Event) {
	if evt.Time() < e.now {
		log.Panic("running event in the past")
	}

	e.InvokeHook(evt, e, BeforeEventHookPos, nil)
	handler := evt.Handler()
	handler.Handle(evt)
	e.InvokeHook(evt, e, AfterEventHookPos, nil)
	e.waitGroup.Done()
}

func (e *ParallelEngine) triggerPostSimulationHandlers() {
	for _, h := range e.postSimHandlers {
		e.waitGroup.Add(1)
		go func() {
			h.Handle(*NewTickEvent(e.now, h))
			e.waitGroup.Done()
		}()
		e.waitGroup.Wait()
	}
}

// CurrentTime returns the current time at which the engine is at. Specifically, the run time of the current event.
func (e *ParallelEngine) CurrentTime() VTimeInSec {
	return e.now
}
