package timing

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
)

// SerialEngine processes scheduled events sequentially in time order.
type SerialEngine struct {
	*hooking.HookableBase

	timeLock sync.RWMutex
	now      VTimeInCycle

	queue          eventQueue
	secondaryQueue eventQueue

	isPaused     bool
	isPausedLock sync.Mutex
	pauseLock    sync.Mutex

	singleRunLock sync.Mutex
}

// NewSerialEngine creates a SerialEngine.
func NewSerialEngine() *SerialEngine {
	return &SerialEngine{
		HookableBase:   hooking.NewHookableBase(),
		queue:          newScheduledEventQueue(),
		secondaryQueue: newScheduledEventQueue(),
	}
}

// Schedule registers an event to be handled in the future.
func (e *SerialEngine) Schedule(evt ScheduledEvent) {
	now := e.readNow()
	if evt.Time < now {
		panic(fmt.Sprintf(
			"timing: cannot schedule event in the past, evt %s @ %d, now %d",
			reflect.TypeOf(evt.Event), evt.Time, now,
		))
	}

	eventCopy := evt
	if evt.IsSecondary {
		e.secondaryQueue.Push(&eventCopy)
		return
	}

	e.queue.Push(&eventCopy)
}

func (e *SerialEngine) readNow() VTimeInCycle {
	e.timeLock.RLock()
	t := e.now
	e.timeLock.RUnlock()
	return t
}

func (e *SerialEngine) writeNow(t VTimeInCycle) {
	e.timeLock.Lock()
	e.now = t
	e.timeLock.Unlock()
}

// Run processes all scheduled events until completion.
func (e *SerialEngine) Run() error {
	e.singleRunLock.Lock()
	defer e.singleRunLock.Unlock()

	for {
		if e.noMoreEvent() {
			return nil
		}

		e.pauseLock.Lock()

		evt := e.nextEvent()
		now := e.readNow()
		if evt.Time < now {
			panic(fmt.Sprintf(
				"timing: cannot run event in the past, evt %s @ %d, now %d",
				reflect.TypeOf(evt.Event), evt.Time, now,
			))
		}

		e.writeNow(evt.Time)

		hookCtx := hooking.HookCtx{
			Domain: e,
			Pos:    HookPosBeforeEvent,
			Item:   evt,
		}
		e.InvokeHook(hookCtx)

		handler := evt.Handler
		if handler != nil {
			_ = handler.Handle(evt.Event)
		}

		hookCtx.Pos = HookPosAfterEvent
		e.InvokeHook(hookCtx)

		e.pauseLock.Unlock()
	}
}

func (e *SerialEngine) noMoreEvent() bool {
	return e.queue.Len() == 0 && e.secondaryQueue.Len() == 0
}

func (e *SerialEngine) nextEvent() *ScheduledEvent {
	if e.queue.Len() == 0 {
		return e.secondaryQueue.Pop()
	}

	if e.secondaryQueue.Len() == 0 {
		return e.queue.Pop()
	}

	primary := e.queue.Peek()
	secondary := e.secondaryQueue.Peek()

	if primary.Time <= secondary.Time {
		e.queue.Pop()
		return primary
	}

	e.secondaryQueue.Pop()
	return secondary
}

// Pause prevents the engine from dispatching more events until Continue is called.
func (e *SerialEngine) Pause() {
	e.isPausedLock.Lock()
	defer e.isPausedLock.Unlock()

	if e.isPaused {
		return
	}

	e.pauseLock.Lock()
	e.isPaused = true
}

// Continue resumes event processing after a Pause.
func (e *SerialEngine) Continue() {
	e.isPausedLock.Lock()
	defer e.isPausedLock.Unlock()

	if !e.isPaused {
		return
	}

	e.pauseLock.Unlock()
	e.isPaused = false
}

// CurrentTime returns the cycle of the most recently executed event.
func (e *SerialEngine) CurrentTime() VTimeInCycle {
	return e.readNow()
}
