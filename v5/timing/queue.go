package timing

import (
	"container/heap"
	"sync"
)

type eventQueue struct {
	sync.Mutex
	events eventHeap
}

func newEventQueue() *eventQueue {
	q := &eventQueue{}
	q.events = make([]Event, 0)
	heap.Init(&q.events)
	return q
}

func (q *eventQueue) Push(evt Event) {
	q.Lock()
	heap.Push(&q.events, evt)
	q.Unlock()
}

func (q *eventQueue) Pop() Event {
	q.Lock()
	defer q.Unlock()
	if q.events.Len() == 0 {
		return nil
	}
	return heap.Pop(&q.events).(Event)
}

func (q *eventQueue) Len() int {
	q.Lock()
	defer q.Unlock()
	return q.events.Len()
}

func (q *eventQueue) Peek() Event {
	q.Lock()
	defer q.Unlock()
	if q.events.Len() == 0 {
		return nil
	}
	return q.events[0]
}

type eventHeap []Event

func (h eventHeap) Len() int { return len(h) }

func (h eventHeap) Less(i, j int) bool {
	return h[i].Time() < h[j].Time()
}

func (h eventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *eventHeap) Push(x any) {
	evt := x.(Event)
	*h = append(*h, evt)
}

func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	evt := old[n-1]
	*h = old[:n-1]
	return evt
}
