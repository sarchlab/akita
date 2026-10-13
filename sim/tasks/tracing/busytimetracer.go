package tracing

import (
	"sort"
	"sync"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type taskTimeStartEnd struct{ start, end timing.VTimeInPicoSec }

// BusyTimeTracer measures the union of complete intervals received at task end.
// It counts time before subscription/restore when an in-flight task later ends.
// Open tasks contribute only once they end. Disjoint completed intervals are
// retained so a later completion can overlap any previously counted interval.
type BusyTimeTracer struct {
	tasks.NopObserver
	lock      sync.Mutex
	filter    TaskFilter
	intervals []taskTimeStartEnd // sorted, nonoverlapping
	busyTime  timing.VTimeInPicoSec
}

// NewBusyTimeTracer creates a tracer for the selected tasks.
func NewBusyTimeTracer(filter TaskFilter) *BusyTimeTracer {
	return &BusyTimeTracer{filter: filter}
}

// BusyTime returns the total time covered by completed, selected tasks.
func (t *BusyTimeTracer) BusyTime() timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.busyTime
}

// TaskEnded merges the full interval even when its start was not observed.
func (t *BusyTimeTracer) TaskEnded(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if (t.filter != nil && !t.filter(task)) || task.EndTime <= task.StartTime {
		return
	}
	interval := taskTimeStartEnd{start: task.StartTime, end: task.EndTime}
	first := sort.Search(len(t.intervals), func(i int) bool { return t.intervals[i].end >= interval.start })
	last := first
	for last < len(t.intervals) && t.intervals[last].start <= interval.end {
		old := t.intervals[last]
		interval.start = min(interval.start, old.start)
		interval.end = max(interval.end, old.end)
		t.busyTime -= old.end - old.start
		last++
	}
	t.busyTime += interval.end - interval.start
	if first == last {
		t.intervals = append(t.intervals, taskTimeStartEnd{})
		copy(t.intervals[first+1:], t.intervals[first:])
	} else {
		copy(t.intervals[first+1:], t.intervals[last:])
		t.intervals = t.intervals[:len(t.intervals)-(last-first)+1]
	}
	t.intervals[first] = interval
}
