package timing

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/sarchlab/akita/v5/hooking"
)

// ParallelEngine dispatches different handlers concurrently at the earliest
// virtual time. Events for one handler run in queue order, never concurrently.
// Scheduling uses short critical sections; workers never borrow dispatcher queues.
type ParallelEngine struct {
	hooking.HookableBase
	mu             sync.Mutex
	now            VTimeInPicoSec
	queue          *unsafeEventQueue
	secondaryQueue *unsafeEventQueue
	registry       map[string]Handler
	supervisor     *Supervisor
	ids            IDGenerator
	wake           chan struct{}
}

func NewParallelEngine() *ParallelEngine {
	return &ParallelEngine{
		queue: newUnsafeEventQueue(), secondaryQueue: newUnsafeEventQueue(),
		registry: make(map[string]Handler), supervisor: NewSupervisor("Engine"),
		ids: NewIDGenerator(), wake: make(chan struct{}, 1),
	}
}
func (e *ParallelEngine) Name() string             { return "Engine" }
func (e *ParallelEngine) Supervisor() *Supervisor  { return e.supervisor }
func (e *ParallelEngine) IDGenerator() IDGenerator { return e.ids }
func (e *ParallelEngine) CurrentTime() VTimeInPicoSec {
	return VTimeInPicoSec(atomic.LoadUint64((*uint64)(&e.now)))
}

func (e *ParallelEngine) RegisterHandler(name string, h Handler) {
	e.supervisor.Check()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry[name] = h
}
func (e *ParallelEngine) Schedule(evt Event) {
	e.supervisor.Check()
	e.mu.Lock()
	defer e.mu.Unlock()
	defer func() {
		select {
		case e.wake <- struct{}{}:
		default:
		}
	}()
	if evt.Time() < e.now {
		panic("cannot schedule event in the past")
	}
	if evt.IsSecondary() {
		e.secondaryQueue.Push(evt)
	} else {
		e.queue.Push(evt)
	}
}
func (e *ParallelEngine) Run() error {
	return e.supervisor.Execute("run", func() error {
		for e.supervisor.awaitResume() {
			groups := e.nextRound()
			if len(groups) == 0 {
				if e.supervisor.waitForWork(e.wake) {
					continue
				}
				break
			}
			var round sync.WaitGroup
			for _, events := range groups {
				round.Add(1)
				err := e.supervisor.Go("event worker", func(context.Context) error {
					defer round.Done()
					for _, evt := range events {
						if e.supervisor.stopped.Load() {
							break
						}
						e.supervisor.Protect("event", evt, func() error { e.dispatch(evt); return nil })
					}
					return nil
				})
				if err != nil {
					round.Done()
					break
				}
			}
			round.Wait()
		}
		return nil
	})
}
func (e *ParallelEngine) nextRound() [][]Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	q := e.queue
	if q.Len() == 0 || (e.secondaryQueue.Len() > 0 && e.secondaryQueue.Peek().Time() < q.Peek().Time()) {
		q = e.secondaryQueue
	}
	if q.Len() == 0 {
		return nil
	}
	atomic.StoreUint64((*uint64)(&e.now), uint64(q.Peek().Time()))
	var groups [][]Event
	indices := make(map[string]int)
	for q.Len() > 0 && q.Peek().Time() == e.now {
		evt := q.Pop()
		i, ok := indices[evt.HandlerID()]
		if !ok {
			i = len(groups)
			indices[evt.HandlerID()] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], evt)
	}
	return groups
}
func (e *ParallelEngine) dispatch(evt Event) {
	if !e.supervisor.beginDispatch() {
		return
	}
	defer e.supervisor.endDispatch()
	ctx := hooking.HookCtx{Domain: e, Pos: HookPosBeforeEvent, Item: evt}
	e.InvokeHook(ctx)
	if e.supervisor.stopped.Load() {
		return
	}
	name := evt.HandlerID()
	e.mu.Lock()
	h := e.registry[name]
	e.mu.Unlock()
	h.Handle(evt)
	if !e.supervisor.stopped.Load() {
		ctx.Pos = HookPosAfterEvent
		e.InvokeHook(ctx)
	}
}
func (e *ParallelEngine) Pause()    { e.supervisor.Pause() }
func (e *ParallelEngine) Continue() { e.supervisor.Continue() }
