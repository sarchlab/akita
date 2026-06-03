package timing

import "testing"

// benchEvent is a minimal Event used by the queue benchmarks. Pointers to a
// fixed pool are reused so the benchmark loop itself allocates nothing, leaving
// only the queue's own allocations visible in allocs/op.
type benchEvent struct {
	t VTimeInSec
}

func (e *benchEvent) Time() VTimeInSec  { return e.t }
func (e *benchEvent) HandlerID() string { return "" }
func (e *benchEvent) IsSecondary() bool { return false }

// benchPushPop fills a queue to depth (with same-time clusters), then repeatedly
// pops the earliest event and reschedules it into the future, holding the queue
// at steady-state depth.
func benchPushPop(b *testing.B, push func(Event), pop func() Event) {
	const depth = 1024

	pool := make([]*benchEvent, depth)
	for i := range pool {
		pool[i] = &benchEvent{t: VTimeInSec(i % 64)} // ~16 events per time
		push(pool[i])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := pop().(*benchEvent)
		e.t = e.t + 64
		push(e)
	}
}

func BenchmarkUnsafeEventQueue(b *testing.B) {
	q := newUnsafeEventQueue()
	benchPushPop(b, q.Push, q.Pop)
}

func BenchmarkEventQueueImpl(b *testing.B) {
	q := NewEventQueue()
	benchPushPop(b, q.Push, q.Pop)
}
