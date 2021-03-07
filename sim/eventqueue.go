package sim

import (
	"container/heap"
	"container/list"
	"sync"
)

// EventQueue are a queue of event ordered by the time of events
type EventQueue interface {
	Push(evt Event)
	Pop() Event
	Len() int
	Peek() Event
}

// EventQueueImpl provides a thread safe event queue
type EventQueueImpl struct {
	sync.Mutex
	events eventHeap
}

// NewEventQueue creates and returns a newly created EventQueue
func NewEventQueue() *EventQueueImpl {
	q := new(EventQueueImpl)
	q.events = make([]Event, 0)
	heap.Init(&q.events)
	return q
}

// Push adds an event to the event queue
func (q *EventQueueImpl) Push(evt Event) {
	q.Lock()
	heap.Push(&q.events, evt)
	q.Unlock()
}

// Pop returns the next earliest event
func (q *EventQueueImpl) Pop() Event {
	q.Lock()
	e := heap.Pop(&q.events).(Event)
	q.Unlock()
	return e
}

// Len returns the number of event in the queue
func (q *EventQueueImpl) Len() int {
	q.Lock()
	l := q.events.Len()
	q.Unlock()
	return l
}

// Peek returns the event in front of the queue without removing it from the
// queue
func (q *EventQueueImpl) Peek() Event {
	q.Lock()
	evt := q.events[0]
	q.Unlock()
	return evt
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

// InsertionQueue is a queue that is based on insertion sort
type InsertionQueue struct {
	lock sync.RWMutex
	l    *list.List
}

// NewInsertionQueue returns a new InsertionQueue
func NewInsertionQueue() *InsertionQueue {
	q := new(InsertionQueue)
	q.l = list.New()
	return q
}

// Push add an event to the event queue
func (q *InsertionQueue) Push(evt Event) {
	var ele *list.Element

	q.lock.RLock()
	for ele = q.l.Front(); ele != nil; ele = ele.Next() {
		if ele.Value.(Event).Time() > evt.Time() {
			break
		}
	}
	q.lock.RUnlock()

	// Insertion
	q.lock.Lock()
	if ele != nil {
		q.l.InsertBefore(evt, ele)
	} else {
		q.l.PushBack(evt)
	}
	q.lock.Unlock()
}

// Pop returns the event with the smallest time, and removes it from the queue
func (q *InsertionQueue) Pop() Event {
	q.lock.Lock()
	evt := q.l.Remove(q.l.Front())
	q.lock.Unlock()
	return evt.(Event)
}

// Len return the number of events in the queue
func (q *InsertionQueue) Len() int {
	q.lock.RLock()
	l := q.l.Len()
	q.lock.RUnlock()
	return l
}

// Peek returns the event at the front of the queue without removing it from
// the queue.
func (q *InsertionQueue) Peek() Event {
	q.lock.RLock()
	evt := q.l.Front().Value.(Event)
	q.lock.RUnlock()
	return evt
}
