package akita

import (
	"log"
	"math"
	"reflect"
	"sync"

	"runtime"
)

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	HookableBase

	pauseLock sync.Mutex
	nowLock   sync.RWMutex
	now       VTimeInSec

	eventChan    chan Event
	waitGroup    sync.WaitGroup
	maxGoRoutine int

	queues    []EventQueue
	queueChan chan EventQueue

	simulationEndHandlers []SimulationEndHandler
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)
	// e.HookableBase = NewHookableBase()

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
		now := e.readNow()

		hookCtx := HookCtx{
			Domain: e,
			Now:    now,
			Pos:    HookPosBeforeEvent,
			Item:   evt,
		}
		e.InvokeHook(&hookCtx)

		handler := evt.Handler()
		handler.Handle(evt)

		hookCtx.Pos = HookPosAfterEvent
		e.InvokeHook(&hookCtx)

		e.waitGroup.Done()
	}
}

func (e *ParallelEngine) readNow() VTimeInSec {
	var now VTimeInSec
	e.nowLock.RLock()
	now = e.now
	e.nowLock.RUnlock()
	return now
}

func (e *ParallelEngine) writeNow(t VTimeInSec) {
	e.nowLock.Lock()
	e.now = t
	e.nowLock.Unlock()
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	now := e.readNow()
	if evt.Time() < now {
		log.Panicf(
			"cannot schedule event in the past, evt %s @ %.10f, now %.10f",
			reflect.TypeOf(evt), evt.Time(), now)
	}

	if evt.Time() == now && now != 0 {
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
			return nil
		}

		e.pauseLock.Lock()
		e.emptyQueueChan()
		e.runEventsUntilConflict()
		e.waitGroup.Wait()
		e.pauseLock.Unlock()
	}
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
	e.writeNow(triggerTime)

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
	now := e.readNow()

	if evt.Time() < now {
		log.Panic("running event in the past")
	}

	hookCtx := HookCtx{
		Domain: e,
		Now:    now,
		Pos:    HookPosBeforeEvent,
		Item:   evt,
	}
	e.InvokeHook(&hookCtx)

	handler := evt.Handler()
	handler.Handle(evt)

	hookCtx.Pos = HookPosAfterEvent
	e.InvokeHook(&hookCtx)

	e.waitGroup.Done()
}

// Pause will prevent the engine to move forward. For events that is scheduled
// at the same time, they may still be triggered.
func (e *ParallelEngine) Pause() {
	e.pauseLock.Lock()
}

// Continue allows the engine to continue to make progress.
func (e *ParallelEngine) Continue() {
	e.pauseLock.Unlock()
}

// CurrentTime returns the current time at which the engine is at.
// Specifically, the run time of the current event.
func (e *ParallelEngine) CurrentTime() VTimeInSec {
	return e.readNow()
}

// RegisterSimulationEndHandler registers a handler to be called after the
// simulation ends.
func (e *ParallelEngine) RegisterSimulationEndHandler(
	handler SimulationEndHandler,
) {
	e.simulationEndHandlers = append(e.simulationEndHandlers, handler)
}

// Finished should be called after the simulation compeletes. It calls
// all the registered SimulationEndHandler
func (e *ParallelEngine) Finished() {
	now := e.readNow()
	for _, h := range e.simulationEndHandlers {
		h.Handle(now)
	}
}
