package event

import "container/heap"

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	queueChan chan *EventQueue
}

// NewSerialEngine creates a SerialEngine
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)

	queue := make(EventQueue, 0, 0)
	heap.Init(&queue)

	e.queueChan = make(chan *EventQueue, 1)
	e.queueChan <- &queue

	return e
}

// Schedule register an event to be happen in the future
func (e *SerialEngine) Schedule(evt Event) {
	queue := <-e.queueChan
	heap.Push(queue, evt)
	e.queueChan <- queue
}

// Run processes all the events scheduled in the SerialEngine
func (e *SerialEngine) Run() error {
	for true {
		queue := <-e.queueChan
		if queue.Len() == 0 {
			return nil
		}

		evt := heap.Pop(queue).(Event)

		e.queueChan <- queue

		handler := evt.Handler()
		go handler.Handle(evt)
		<-evt.FinishChan()
	}
	return nil
}
