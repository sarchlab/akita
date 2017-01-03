package event

// An Event is something going yo happen in ther future.
type Event interface {
	Time() float64
	Happen()
}

// A HandledEvent does not directly triggers something to happen, but it
// relies on handlers to handle it.
type HandledEvent struct {
	time     float64
	handlers []Handler
}

func NewHandledEvent(time float64) *HandledEvent {
	e := new(HandledEvent)
	e.time = time
	e.handlers = make([]Handler, 0)
	return e
}

func (e *HandledEvent) AddHandler(h Handler) {
	e.handlers = append(e.handlers, h)
}

func (e *HandledEvent) Happen() {
	for _, handler := range e.handlers {
		handler.Handle(e)
	}
}

func (e HandledEvent) Time() float64 {
	return e.time
}

type Handler interface {
	Handle(e Event)
}
