package event

import "container/heap"

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	queue     EventQueue
	eventIn   chan Event
	queueChan chan EventQueue
}

// NewSerialEngine creates a SerialEngine
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)

	e.queue = make(EventQueue, 0, 1000)
	heap.Init(&e.queue)
	e.eventIn = make(chan Event)
	e.queueChan = make(chan EventQueue, 1)
	e.queueChan <- e.queue

	go e.schedule()

	return e
}

func (e *SerialEngine) schedule() {
	for evt := range e.eventIn {
		eq := <-e.queueChan
		heap.Push(&eq, evt)
		e.queueChan <- eq
	}
}

// EventChan returns a channel of events. Others can send to the channel to
// request scheduling events
func (e *SerialEngine) EventChan() chan Event {
	return e.eventIn
}

// Run processes all the events scheduled in the SerialEngine
func (e *SerialEngine) Run() {
	for true {
		eq := <-e.queueChan
		evt := heap.Pop(&eq).(Event)
		go evt.Handler().Handle(evt)
		e.queueChan <- eq
		<-evt.FinishChan()
	}
}
