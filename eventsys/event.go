package eventsys

// An Event is something going to happen in the future.
type Event interface {
	SetTime(time float64)
	Time() float64
	Happen()
}

// A HandledEvent does not directly triggers something to happen, but it
// relies on handlers to handle it.
type HandledEvent struct {
	time     float64
	handlers []Handler
}

// NewHandledEvent creates a new handled event
func NewHandledEvent() *HandledEvent {
	e := new(HandledEvent)
	e.handlers = make([]Handler, 0, 1)
	return e
}

// AddHandler register a handler to the event. When the event happens, all
// the handlers will be involked to handle the event. There is no gurantee
// on the order of which handler got invoked first.
func (e *HandledEvent) AddHandler(h Handler) {
	e.handlers = append(e.handlers, h)
}

// Happen of a HandledEvent will invoke all the handlers to handle the event.
func (e *HandledEvent) Happen() {
	for _, handler := range e.handlers {
		handler.Handle(e)
	}
}

func (e *HandledEvent) SetTime(time float64) {
	e.time = time
}

func (e HandledEvent) Time() float64 {
	return e.time
}

// A Handler defines the action that is associated with a HandledEvent. When
// a handled event happen, the handles attached with the event will be invoked.
type Handler interface {
	Handle(e Event)
}
