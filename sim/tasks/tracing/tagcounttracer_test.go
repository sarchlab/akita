package tracing

import (
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/sim/tasks"
)

func TestTagCountTracerConcurrentLifecycles(t *testing.T) {
	tracer := NewTagCountTracer(nil)
	const count = 64
	var group sync.WaitGroup
	for id := uint64(1); id <= count; id++ {
		group.Add(1)
		go func() {
			defer group.Done()
			task := tasks.Task{ID: id}
			tracer.TaskStarted(task)
			tracer.TagAdded(tasks.Tag{TaskID: id, What: "hit"})
			tracer.TagAdded(tasks.Tag{TaskID: id, What: "hit"})
			tracer.TaskEnded(task)
			// Queries must remain safe during and after completion callbacks.
			tracer.TagNames()
			tracer.TagCount("hit")
			tracer.TaskCount("hit")
		}()
	}
	group.Wait()
	if got := tracer.TagCount("hit"); got != 2*count {
		t.Fatalf("tag count = %d, want %d", got, 2*count)
	}
	if got := tracer.TaskCount("hit"); got != count {
		t.Fatalf("task count = %d, want %d", got, count)
	}
	if len(tracer.inflightTasks) != 0 {
		t.Fatal("completed tasks retained")
	}
	names := tracer.TagNames()
	names[0] = "mutated"
	if tracer.TagNames()[0] != "hit" {
		t.Fatal("TagNames exposed internal storage")
	}
}
