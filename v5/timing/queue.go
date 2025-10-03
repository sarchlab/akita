package timing

import (
	"container/heap"
	"sync"
)

type eventQueue interface {
	Push(*FutureEvent)
	Pop() *FutureEvent
	Len() int
	Peek() *FutureEvent
}

type futureEventQueue struct {
	sync.Mutex
	events futureEventHeap
}

func newFutureEventQueue() *futureEventQueue {
	q := &futureEventQueue{}
	q.events = make([]*FutureEvent, 0)
	heap.Init(&q.events)
	return q
}

func (q *futureEventQueue) Push(evt *FutureEvent) {
	q.Lock()
	heap.Push(&q.events, evt)
	q.Unlock()
}

func (q *futureEventQueue) Pop() *FutureEvent {
	q.Lock()
	defer q.Unlock()
	if q.events.Len() == 0 {
		return nil
	}
	return heap.Pop(&q.events).(*FutureEvent)
}

func (q *futureEventQueue) Len() int {
	q.Lock()
	defer q.Unlock()
	return q.events.Len()
}

func (q *futureEventQueue) Peek() *FutureEvent {
	q.Lock()
	defer q.Unlock()
	if q.events.Len() == 0 {
		return nil
	}
	return q.events[0]
}

type futureEventHeap []*FutureEvent

func (h futureEventHeap) Len() int { return len(h) }

func (h futureEventHeap) Less(i, j int) bool {
	return h[i].Time < h[j].Time
}

func (h futureEventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *futureEventHeap) Push(x any) {
	evt := x.(*FutureEvent)
	*h = append(*h, evt)
}

func (h *futureEventHeap) Pop() any {
	old := *h
	n := len(old)
	evt := old[n-1]
	*h = old[:n-1]
	return evt
}
