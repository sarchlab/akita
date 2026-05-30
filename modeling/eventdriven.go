package modeling

import (
	"encoding/json"
	"io"
	"math"
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// EventProcessor defines the processing logic for an EventDrivenComponent.
// S is the Spec type, T is the State type.
type EventProcessor[S any, T any] interface {
	Process(comp *EventDrivenComponent[S, T], now timing.VTimeInSec) bool
}

// TimerFiredEvent is the event scheduled by EventDrivenComponent to wake
// itself up at a future time.
type TimerFiredEvent struct {
	*timing.EventBase
}

// EventDrivenComponent is a generic component that reacts to events rather
// than ticking at a fixed frequency.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Instead of periodic ticking, EventDrivenComponent schedules wakeup events
// via [ScheduleWakeAt] or [ScheduleWakeNow]. A dedup guard (pendingWakeup)
// prevents redundant event scheduling.
type EventDrivenComponent[S any, T any] struct {
	sync.Mutex
	hooking.HookableBase
	*messaging.PortOwnerBase

	Engine    timing.EventScheduler
	name      string
	Spec      S
	State     T
	processor EventProcessor[S, T]

	pendingWakeup timing.VTimeInSec
}

// Name returns the component name.
func (c *EventDrivenComponent[S, T]) Name() string {
	return c.name
}

// StateRef returns a live reference to the component's runtime state, exposing
// the State field to the simulation's global state manager (it satisfies
// simulation.StateHolder structurally). The returned pointer aliases the State
// field, so reads and writes through it are shared with the component.
func (c *EventDrivenComponent[S, T]) StateRef() any {
	return &c.State
}

// ScheduleWakeAt schedules a wakeup at time t. If a wakeup is already
// pending at the same or earlier time, this is a no-op (dedup guard).
func (c *EventDrivenComponent[S, T]) ScheduleWakeAt(t timing.VTimeInSec) {
	if c.pendingWakeup != math.MaxUint64 && c.pendingWakeup <= t {
		return
	}

	c.pendingWakeup = t

	evt := &TimerFiredEvent{
		EventBase: timing.NewEventBase(t, c.Name()),
	}
	c.Engine.Schedule(evt)
}

// ScheduleWakeNow schedules a wakeup at the current engine time.
func (c *EventDrivenComponent[S, T]) ScheduleWakeNow() {
	c.ScheduleWakeAt(c.Engine.CurrentTime())
}

// Handle processes an event. For TimerFiredEvent, it resets the dedup guard
// and calls the processor.
func (c *EventDrivenComponent[S, T]) Handle(e timing.Event) error {
	c.Lock()
	defer c.Unlock()

	c.pendingWakeup = math.MaxUint64
	c.processor.Process(c, e.Time())

	return nil
}

// NotifyRecv is called when a port receives a message. It schedules an
// immediate wakeup.
func (c *EventDrivenComponent[S, T]) NotifyRecv(port messaging.Port) {
	c.ScheduleWakeNow()
}

// NotifyPortFree is called when a port becomes free. It schedules an
// immediate wakeup.
func (c *EventDrivenComponent[S, T]) NotifyPortFree(port messaging.Port) {
	c.ScheduleWakeNow()
}

// ResetWakeup resets the pending wakeup guard to math.MaxUint64, allowing new
// wakeup events to be scheduled. This is used after loading state from a
// checkpoint.
func (c *EventDrivenComponent[S, T]) ResetWakeup() {
	c.pendingWakeup = math.MaxUint64
}

// SaveState marshals the component's spec and state as JSON and writes
// it to w.
func (c *EventDrivenComponent[S, T]) SaveState(w io.Writer) error {
	snap := componentSnapshot[S, T]{
		Spec:  c.Spec,
		State: c.State,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	_, err = w.Write(data)

	return err
}

// LoadState reads JSON from r and restores the component's spec and state.
func (c *EventDrivenComponent[S, T]) LoadState(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var snap componentSnapshot[S, T]
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	c.Spec = snap.Spec
	c.State = snap.State

	return nil
}
