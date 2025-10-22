package timing

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
)

// ParallelEngine executes events at the same simulation time in parallel.
type ParallelEngine struct {
	*hooking.HookableBase

	pauseLock sync.Mutex

	nowLock sync.RWMutex
	now     VTimeInCycle

	waitGroup sync.WaitGroup

	queues    []*eventQueue
	queueChan chan *eventQueue
}

// NewParallelEngine creates a ParallelEngine.
func NewParallelEngine() *ParallelEngine {
	numQueues := runtime.GOMAXPROCS(0)

	engine := &ParallelEngine{
		HookableBase: hooking.NewHookableBase(),
		queues:       make([]*eventQueue, 0, numQueues),
		queueChan:    make(chan *eventQueue, numQueues),
	}

	for i := 0; i < numQueues; i++ {
		q := newEventQueue()

		engine.queueChan <- q

		engine.queues = append(engine.queues, q)
	}

	return engine
}

func (e *ParallelEngine) readNow() VTimeInCycle {
	e.nowLock.RLock()
	t := e.now
	e.nowLock.RUnlock()
	return t
}

func (e *ParallelEngine) writeNow(t VTimeInCycle) {
	e.nowLock.Lock()
	e.now = t
	e.nowLock.Unlock()
}

// Schedule registers an event to be processed by the engine.
func (e *ParallelEngine) Schedule(evt Event) {
	now := e.readNow()
	if evt.Time() < now {
		panic(fmt.Sprintf(
			"timing: cannot schedule event in the past, evt %s @ %d, now %d",
			reflect.TypeOf(evt), evt.Time(), now,
		))
	}

	queue := <-e.queueChan
	queue.Push(evt)
	e.queueChan <- queue
}

// Run processes all scheduled events until the queues drain.
func (e *ParallelEngine) Run() error {
	for {
		if !e.hasMoreEvents() {
			return nil
		}

		e.pauseLock.Lock()
		e.determineWhatToRun()
		e.runRound()
		e.pauseLock.Unlock()
	}
}

func (e *ParallelEngine) determineWhatToRun() {
	e.writeNow(e.earliestTimeInQueueGroup(e.queues))
}

func (e *ParallelEngine) earliestTimeInQueueGroup(queues []*eventQueue) VTimeInCycle {
	earliest := maxCycleValue

	for _, q := range queues {
		if q.Len() == 0 {
			continue
		}
		if t := q.Peek().Time(); t < earliest {
			earliest = t
		}
	}

	return earliest
}

func (e *ParallelEngine) runRound() {
	e.emptyQueueChan(e.queues, e.queueChan)
	e.runEventsUntilConflict(e.queues, e.queueChan)
	e.waitGroup.Wait()
}

func (e *ParallelEngine) emptyQueueChan(queues []*eventQueue, ch chan *eventQueue) {
	for range queues {
		<-ch
	}
}

func (e *ParallelEngine) hasMoreEvents() bool {
	return e.hasMoreInGroup(e.queues)
}

func (e *ParallelEngine) hasMoreInGroup(queues []*eventQueue) bool {
	for _, q := range queues {
		if q.Len() > 0 {
			return true
		}
	}
	return false
}

func (e *ParallelEngine) runEventsUntilConflict(queues []*eventQueue, ch chan *eventQueue) {
	now := e.readNow()

	for _, queue := range queues {
		for queue.Len() > 0 {
			evt := queue.Peek()
			switch {
			case evt.Time() == now:
				queue.Pop()
				e.runEventWithTempWorker(evt)
			case evt.Time() < now:
				panic(fmt.Sprintf(
					"timing: cannot run event in the past, evt %s @ %d, now %d",
					reflect.TypeOf(evt), evt.Time(), now,
				))
			default:
				// future event, leave in queue
				goto nextQueue
			}
		}
	nextQueue:
		ch <- queue
	}
}

func (e *ParallelEngine) runEventWithTempWorker(evt Event) {
	e.waitGroup.Add(1)
	go e.tempWorkerRun(evt)
}

func (e *ParallelEngine) tempWorkerRun(evt Event) {
	defer e.waitGroup.Done()

	now := e.readNow()
	if evt.Time() < now {
		panic("timing: running event in the past")
	}

	hookCtx := hooking.HookCtx{
		Domain: e,
		Pos:    HookPosBeforeEvent,
		Item:   evt,
	}
	e.InvokeHook(hookCtx)

	handler := evt.Handler()
	if handler != nil {
		_ = handler.Handle(evt)
	}

	hookCtx.Pos = HookPosAfterEvent
	e.InvokeHook(hookCtx)
}

// Pause prevents the engine from progressing to future times.
func (e *ParallelEngine) Pause() {
	e.pauseLock.Lock()
}

// Continue allows the engine to resume progress.
func (e *ParallelEngine) Continue() {
	e.pauseLock.Unlock()
}

// CurrentTime returns the most recent simulation cycle processed.
func (e *ParallelEngine) CurrentTime() VTimeInCycle {
	return e.readNow()
}

// Ensure ParallelEngine exposes the scheduling API components depend on.
type parallelScheduler interface {
	Schedule(Event)
	CurrentTime() VTimeInCycle
}

var _ parallelScheduler = (*ParallelEngine)(nil)
