package core

import (
	"container/heap"
	"sync"
)

// EventQueue is a priority queue of events. The front of the queue is always
// the event to happen next
type EventQueue struct {
	sync.Mutex
	events []Event
}

// NewEventQueue creates and returns a newly created EventQueue
func NewEventQueue() *EventQueue {
	q := new(EventQueue)
	q.events = make([]Event, 0, 0)
	heap.Init(q)
	return q
}

// Len returns the length of the event queue
func (eq *EventQueue) Len() int {
	eq.Lock()
	defer eq.Unlock()
	return len(eq.events)
}

// Less determines the order between two events. Less returns true if the i-th
// event happens before the j-th event.
func (eq *EventQueue) Less(i, j int) bool {
	eq.Lock()
	defer eq.Unlock()
	return eq.events[i].Time() < eq.events[j].Time()
}

// Swap changes the position of two events in the event queue
func (eq *EventQueue) Swap(i, j int) {
	eq.Lock()
	defer eq.Unlock()
	eq.events[i], eq.events[j] = eq.events[j], eq.events[i]
}

// Push adds an event into the event queue
func (eq *EventQueue) Push(x interface{}) {
	eq.Lock()
	defer eq.Unlock()
	event := x.(Event)
	eq.events = append(eq.events, event)
}

// Pop removes and returns the next event to happen
func (eq *EventQueue) Pop() interface{} {
	eq.Lock()
	defer eq.Unlock()
	old := eq.events
	n := len(old)
	event := old[n-1]
	eq.events = old[0 : n-1]
	return event
}
