package timing

import (
	"container/heap"
	"sync"
)

type eventQueue interface {
	Push(*ScheduledEvent)
	Pop() *ScheduledEvent
	Len() int
	Peek() *ScheduledEvent
}

type scheduledEventQueue struct {
	sync.Mutex
	events scheduledEventHeap
}

func newScheduledEventQueue() *scheduledEventQueue {
	q := &scheduledEventQueue{}
	q.events = make([]*ScheduledEvent, 0)
	heap.Init(&q.events)
	return q
}

func (q *scheduledEventQueue) Push(evt *ScheduledEvent) {
	q.Lock()
	heap.Push(&q.events, evt)
	q.Unlock()
}

func (q *scheduledEventQueue) Pop() *ScheduledEvent {
	q.Lock()
	defer q.Unlock()
	if q.events.Len() == 0 {
		return nil
	}
	return heap.Pop(&q.events).(*ScheduledEvent)
}

func (q *scheduledEventQueue) Len() int {
	q.Lock()
	defer q.Unlock()
	return q.events.Len()
}

func (q *scheduledEventQueue) Peek() *ScheduledEvent {
	q.Lock()
	defer q.Unlock()
	if q.events.Len() == 0 {
		return nil
	}
	return q.events[0]
}

type scheduledEventHeap []*ScheduledEvent

func (h scheduledEventHeap) Len() int { return len(h) }

func (h scheduledEventHeap) Less(i, j int) bool {
	return h[i].Time < h[j].Time
}

func (h scheduledEventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *scheduledEventHeap) Push(x any) {
	evt := x.(*ScheduledEvent)
	*h = append(*h, evt)
}

func (h *scheduledEventHeap) Pop() any {
	old := *h
	n := len(old)
	evt := old[n-1]
	*h = old[:n-1]
	return evt
}
