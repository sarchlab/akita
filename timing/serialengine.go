package timing

import (
	"log"
	"sync/atomic"

	"github.com/sarchlab/akita/v5/hooking"
)

// A SerialEngine runs events one after another on the goroutine calling Run.
// During execution, only that goroutine may schedule events or mutate engine
// state. External callers may Pause/Continue, read CurrentTime, or inspect the
// supervisor. Other access requires stopped execution and host coordination.
type SerialEngine struct {
	hooking.HookableBase

	time           VTimeInPicoSec
	queue          *unsafeEventQueue
	secondaryQueue *unsafeEventQueue

	supervisor *Supervisor
	ids        IDGenerator

	registry map[string]Handler
	restored bool
}

// NewSerialEngine creates a SerialEngine.
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)

	e.queue = newUnsafeEventQueue()
	e.secondaryQueue = newUnsafeEventQueue()
	e.registry = make(map[string]Handler)
	e.supervisor = NewSupervisor("Engine")
	e.supervisor.serial = true
	e.ids = NewIDGenerator()

	return e
}

// Name returns the name of the engine. The engine is registered as a simulation
// entity so its event-queue and time state are part of the state snapshot.
func (e *SerialEngine) Name() string {
	return "Engine"
}

// RegisterHandler registers a handler with the given name.
func (e *SerialEngine) RegisterHandler(name string, handler Handler) {
	e.supervisor.Check()
	e.registry[name] = handler
}

// Schedule registers an event to happen in the future. During a run, only
// callbacks on the run goroutine may call Schedule; background producers cannot.
func (e *SerialEngine) Schedule(evt Event) {
	e.supervisor.Check()
	if evt.Time() < e.time {
		log.Panic("scheduling an event earlier than current time")
	}

	if evt.IsSecondary() {
		e.secondaryQueue.Push(evt)

		return
	}

	e.queue.Push(evt)
}

// Run processes all the events scheduled in the SerialEngine.
func (e *SerialEngine) Run() error { return e.run(nil) }

// RunUntil processes events at or before t, retaining later events for another run.
func (e *SerialEngine) RunUntil(t VTimeInPicoSec) error { return e.run(&t) }

func (e *SerialEngine) run(limit *VTimeInPicoSec) error {
	return e.supervisor.Execute("run", func() error {
		hasHooks := e.NumHooks() > 0
		for e.supervisor.awaitResume() {
			if e.dispatchNext(hasHooks, limit) {
				break
			}
		}
		return nil
	})
}

// dispatchNext accesses the queue and model on the same goroutine.
// A recovery is installed before calling even user-defined event accessors.
func (e *SerialEngine) dispatchNext(hasHooks bool, limit *VTimeInPicoSec) (done bool) {
	var evt Event
	defer func() {
		if p := recover(); p != nil {
			e.supervisor.record("event", evt, p)
		}
	}()
	var handler Handler
	evt, handler, done = e.prepareNext(limit)
	if done {
		return true
	}
	ctx := hooking.HookCtx{Domain: e, Pos: HookPosBeforeEvent, Item: evt}
	if hasHooks {
		e.InvokeHook(ctx)
	}
	if e.supervisor.stopped.Load() {
		return false
	}
	handler.Handle(evt)
	if hasHooks && !e.supervisor.stopped.Load() {
		ctx.Pos = HookPosAfterEvent
		e.InvokeHook(ctx)
	}
	return false
}

func (e *SerialEngine) prepareNext(limit *VTimeInPicoSec) (Event, Handler, bool) {
	if e.noMoreEvent() || (limit != nil && e.nextEventTime() > *limit) {
		return nil, nil, true
	}
	evt := e.nextEvent()
	if evt.Time() < e.time {
		panic("cannot run event in the past")
	}
	atomic.StoreUint64((*uint64)(&e.time), uint64(evt.Time()))
	return evt, e.registry[evt.HandlerID()], false
}

// nextEventTime returns the time of the earliest queued event. It must not be
// called when both queues are empty.
func (e *SerialEngine) nextEventTime() VTimeInPicoSec {
	if e.queue.Len() == 0 {
		return e.secondaryQueue.Peek().Time()
	}
	if e.secondaryQueue.Len() == 0 {
		return e.queue.Peek().Time()
	}

	primary := e.queue.Peek().Time()
	secondary := e.secondaryQueue.Peek().Time()
	if primary <= secondary {
		return primary
	}

	return secondary
}

func (e *SerialEngine) noMoreEvent() bool {
	return e.queue.Len() == 0 && e.secondaryQueue.Len() == 0
}

func (e *SerialEngine) nextEvent() Event {
	if e.queue.Len() == 0 {
		return e.secondaryQueue.Pop()
	}

	if e.secondaryQueue.Len() == 0 {
		return e.queue.Pop()
	}

	primaryEvt := e.queue.Peek()
	secondaryEvt := e.secondaryQueue.Peek()

	if primaryEvt.Time() <= secondaryEvt.Time() {
		e.queue.Pop()
		return primaryEvt
	}

	e.secondaryQueue.Pop()

	return secondaryEvt
}

// Pause prevents subsequent event dispatch; an already admitted event may finish.
func (e *SerialEngine) Pause()    { e.supervisor.Pause() }
func (e *SerialEngine) Continue() { e.supervisor.Continue() }
func (e *SerialEngine) CurrentTime() VTimeInPicoSec {
	return VTimeInPicoSec(atomic.LoadUint64((*uint64)(&e.time)))
}
func (e *SerialEngine) SetCurrentTime(t VTimeInPicoSec) {
	e.supervisor.Check()
	atomic.StoreUint64((*uint64)(&e.time), uint64(t))
}
func (e *SerialEngine) Supervisor() *Supervisor  { return e.supervisor }
func (e *SerialEngine) IDGenerator() IDGenerator { return e.ids }
