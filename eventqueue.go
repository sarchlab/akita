package core

import (
	"container/heap"
	"sync"
)

// EventQueue is a priority queue of events. The front of the queue is always
// the event to happen next
type EventQueue struct {
	sync.Mutex
	events eventHeap
}

// NewEventQueue creates and returns a newly created EventQueue
func NewEventQueue() *EventQueue {
	q := new(EventQueue)
	q.events = make([]Event, 0, 0)
	heap.Init(&q.events)
	return q
}

// Push adds an event to the event queue
func (q *EventQueue) Push(evt Event) {
	q.Lock()
	defer q.Unlock()
	heap.Push(&q.events, evt)
}

// Pop returns the next earliest event
func (q *EventQueue) Pop() Event {
	q.Lock()
	defer q.Unlock()
	return heap.Pop(&q.events).(Event)
}

// Len returns the number of event in the queue
func (q *EventQueue) Len() int {
	q.Lock()
	defer q.Unlock()
	return len(q.events)
}

type eventHeap []Event

// Len returns the length of the event queue
func (h eventHeap) Len() int {
	return len(h)
}

// Less determines the order between two events. Less returns true if the i-th
// event happens before the j-th event.
func (h eventHeap) Less(i, j int) bool {
	return h[i].Time() < h[j].Time()
}

// Swap changes the position of two events in the event queue
func (h eventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Push adds an event into the event queue
func (h *eventHeap) Push(x interface{}) {
	event := x.(Event)
	*h = append(*h, event)
}

// Pop removes and returns the next event to happen
func (h *eventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	event := old[n-1]
	*h = old[0 : n-1]
	return event
}
