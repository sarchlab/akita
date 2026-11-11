package timing

import (
	"sort"
	"sync"
	"sync/atomic"
)

// eventQueue is a queue of events ordered by event time. Peek and Pop report
// whether an event was present; empty reads return nil, false.
type eventQueue interface {
	Push(evt Event)
	Pop() (Event, bool)
	Len() int
	Peek() (Event, bool)
}

// queuedEvent pairs an event with the sequence number it was assigned when
// pushed. Events at the same time are ordered by sequence — i.e. in schedule
// order (FIFO) — giving a total, deterministic order that is reproducible after
// a checkpoint is saved and restored.
type queuedEvent struct {
	event      Event
	seq        uint64
	background bool
}

func queueEvent(evt Event, seq uint64) queuedEvent {
	kind, ok := evt.(BackgroundEvent)
	return queuedEvent{event: evt, seq: seq, background: ok && kind.IsBackground()}
}

// eventHeap is a typed binary min-heap of queued events ordered by (time, seq).
// It is implemented directly rather than through container/heap so that pushing
// a queuedEvent never boxes it into an interface{} (which would allocate on
// every schedule).
type eventHeap []queuedEvent

func (h eventHeap) less(i, j int) bool {
	a, b := h[i], h[j]
	if a.event.Time() != b.event.Time() {
		return a.event.Time() < b.event.Time()
	}
	return a.seq < b.seq
}

func (h eventHeap) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			break
		}
		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (h eventHeap) down(i, n int) {
	for {
		left := 2*i + 1
		if left >= n {
			break
		}
		smallest := left
		if right := left + 1; right < n && h.less(right, left) {
			smallest = right
		}
		if !h.less(smallest, i) {
			break
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}

// eventQueueImpl provides a thread-safe event queue.
type eventQueueImpl struct {
	sync.Mutex
	foreground *atomic.Int64 // shared across all parallel-engine queues

	events  eventHeap
	nextSeq uint64
}

// newEventQueue creates and returns a newly created EventQueue.
func newEventQueue(foreground *atomic.Int64) *eventQueueImpl {
	return &eventQueueImpl{foreground: foreground}
}

// Push adds an event to the event queue.
func (q *eventQueueImpl) Push(evt Event) {
	q.Lock()
	defer q.Unlock()
	entry := queueEvent(evt, q.nextSeq)
	q.events = append(q.events, entry)
	if !entry.background {
		q.foreground.Add(1)
	}
	q.nextSeq++
	q.events.up(len(q.events) - 1)
}

// Pop returns the next earliest event.
func (q *eventQueueImpl) Pop() (Event, bool) {
	q.Lock()
	defer q.Unlock()
	if len(q.events) > 0 && !q.events[0].background {
		q.foreground.Add(-1)
	}
	return popHeap(&q.events)
}

// Len returns the number of events in the queue.
func (q *eventQueueImpl) Len() int {
	q.Lock()
	defer q.Unlock()
	l := len(q.events)

	return l
}

// Peek returns the event in front of the queue without removing it from the
// queue.
func (q *eventQueueImpl) Peek() (Event, bool) {
	q.Lock()
	defer q.Unlock()
	if len(q.events) == 0 {
		return nil, false
	}
	return q.events[0].event, true
}

// unsafeEventQueue is a lock-free event queue for single-threaded use.
// It implements the EventQueue interface but without mutex overhead.
// Used by SerialEngine where the Run loop is inherently single-threaded.
type unsafeEventQueue struct {
	foreground int
	events     eventHeap
	nextSeq    uint64
}

// newUnsafeEventQueue creates an unsafeEventQueue for single-threaded use.
func newUnsafeEventQueue() *unsafeEventQueue {
	return &unsafeEventQueue{events: make(eventHeap, 0)}
}

// Push adds an event to the event queue without locking.
func (q *unsafeEventQueue) Push(evt Event) {
	entry := queueEvent(evt, q.nextSeq)
	q.events = append(q.events, entry)
	if !entry.background {
		q.foreground++
	}
	q.nextSeq++
	q.events.up(len(q.events) - 1)
}

// Pop returns the next earliest event without locking.
func (q *unsafeEventQueue) Pop() (Event, bool) {
	if len(q.events) > 0 && !q.events[0].background {
		q.foreground--
	}
	return popHeap(&q.events)
}

// Len returns the number of events in the queue without locking.
func (q *unsafeEventQueue) Len() int {
	return len(q.events)
}

// Peek returns the event in front of the queue without removing it.
func (q *unsafeEventQueue) Peek() (Event, bool) {
	if len(q.events) == 0 {
		return nil, false
	}
	return q.events[0].event, true
}

// snapshot returns the queue's events in pop order — by time, then schedule
// order — without modifying the queue. Used to checkpoint the queue.
func (q *unsafeEventQueue) snapshot() []Event {
	sorted := append(eventHeap(nil), q.events...)
	sort.Slice(sorted, func(i, j int) bool { return sorted.less(i, j) })

	out := make([]Event, len(sorted))
	for i := range sorted {
		out[i] = sorted[i].event
	}

	return out
}

// restore pushes events as if freshly scheduled, re-assigning sequence numbers
// in input order. Given events in pop order, this reproduces the original
// (time, seq) ordering. The queue should be empty.
func (q *unsafeEventQueue) restore(events []Event) {
	for _, e := range events {
		q.Push(e)
	}
}

// popHeap removes and returns the earliest event from the heap.
func popHeap(h *eventHeap) (Event, bool) {
	events := *h
	n := len(events)
	if n == 0 {
		return nil, false
	}
	root := events[0].event

	events[0] = events[n-1]
	events[n-1] = queuedEvent{}
	events = events[:n-1]
	*h = events

	if len(events) > 0 {
		events.down(0, len(events))
	}

	return root, true
}
