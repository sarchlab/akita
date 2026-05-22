package timing

import (
	"container/heap"
	"sync"
)

// EventQueue is a queue of events ordered by event time.
type EventQueue interface {
	Push(evt Event)
	Pop() Event
	Len() int
	Peek() Event
}

// EventQueueImpl provides a thread-safe event queue.
type EventQueueImpl struct {
	sync.Mutex

	events eventHeap
}

// NewEventQueue creates and returns a newly created EventQueue.
func NewEventQueue() *EventQueueImpl {
	q := new(EventQueueImpl)
	q.events = make([]Event, 0)
	heap.Init(&q.events)

	return q
}

// Push adds an event to the event queue.
func (q *EventQueueImpl) Push(evt Event) {
	q.Lock()
	heap.Push(&q.events, evt)
	q.Unlock()
}

// Pop returns the next earliest event.
func (q *EventQueueImpl) Pop() Event {
	q.Lock()
	e := heap.Pop(&q.events).(Event)
	q.Unlock()

	return e
}

// Len returns the number of events in the queue.
func (q *EventQueueImpl) Len() int {
	q.Lock()
	l := q.events.Len()
	q.Unlock()

	return l
}

// Peek returns the event in front of the queue without removing it from the
// queue.
func (q *EventQueueImpl) Peek() Event {
	q.Lock()
	evt := q.events[0]
	q.Unlock()

	return evt
}

type eventHeap []Event

// Len returns the length of the event queue.
func (h eventHeap) Len() int {
	return len(h)
}

// Less determines the order between two events. Less returns true if the i-th
// event happens before the j-th event.
func (h eventHeap) Less(i, j int) bool {
	return h[i].Time() < h[j].Time()
}

// Swap changes the position of two events in the event queue.
func (h eventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Push adds an event into the event queue.
func (h *eventHeap) Push(x interface{}) {
	event := x.(Event)
	*h = append(*h, event)
}

// Pop removes and returns the next event to happen.
func (h *eventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	event := old[n-1]
	*h = old[0 : n-1]

	return event
}

// unsafeEventQueue is a lock-free event queue for single-threaded use.
// It implements the EventQueue interface but without mutex overhead.
// Used by SerialEngine where the Run loop is inherently single-threaded.
type unsafeEventQueue struct {
	events eventHeap
}

// newUnsafeEventQueue creates an unsafeEventQueue for single-threaded use.
func newUnsafeEventQueue() *unsafeEventQueue {
	q := new(unsafeEventQueue)
	q.events = make([]Event, 0)
	heap.Init(&q.events)

	return q
}

// Push adds an event to the event queue without locking.
func (q *unsafeEventQueue) Push(evt Event) {
	heap.Push(&q.events, evt)
}

// Pop returns the next earliest event without locking.
func (q *unsafeEventQueue) Pop() Event {
	return heap.Pop(&q.events).(Event)
}

// Len returns the number of events in the queue without locking.
func (q *unsafeEventQueue) Len() int {
	return q.events.Len()
}

// Peek returns the event in front of the queue without removing it.
func (q *unsafeEventQueue) Peek() Event {
	return q.events[0]
}
