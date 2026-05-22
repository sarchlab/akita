package modeling

import (
	"math"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// EventDrivenBuilder constructs [EventDrivenComponent] instances.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
type EventDrivenBuilder[S any, T any] struct {
	engine    timing.EventScheduler
	spec      S
	processor EventProcessor[S, T]
}

// NewEventDrivenBuilder creates a new EventDrivenBuilder.
func NewEventDrivenBuilder[S any, T any]() EventDrivenBuilder[S, T] {
	return EventDrivenBuilder[S, T]{}
}

// WithEngine sets the simulation engine.
func (b EventDrivenBuilder[S, T]) WithEngine(engine timing.EventScheduler) EventDrivenBuilder[S, T] {
	b.engine = engine
	return b
}

// WithSpec sets the component specification.
func (b EventDrivenBuilder[S, T]) WithSpec(spec S) EventDrivenBuilder[S, T] {
	b.spec = spec
	return b
}

// WithProcessor sets the event processor.
func (b EventDrivenBuilder[S, T]) WithProcessor(
	processor EventProcessor[S, T],
) EventDrivenBuilder[S, T] {
	b.processor = processor
	return b
}

// Build creates the EventDrivenComponent with the given name.
func (b EventDrivenBuilder[S, T]) Build(name string) *EventDrivenComponent[S, T] {
	naming.MustBeValid(name)

	comp := &EventDrivenComponent[S, T]{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		Engine:        b.engine,
		name:          name,
		Spec:          b.spec,
		processor:     b.processor,
		pendingWakeup: math.MaxUint64,
	}

	if registrar, ok := b.engine.(timing.HandlerRegistrar); ok {
		registrar.RegisterHandler(name, comp)
	}

	return comp
}
