package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim/tasks"
)

// TagCountTracer counts how often each tag name is recorded, and how many
// distinct tasks carry a tag with a given name.
type TagCountTracer struct {
	tasks.NopObserver

	filter           TaskFilter
	lock             sync.Mutex
	inflightTasks    map[uint64]map[string]bool // task ID -> set of seen tag names
	tagNames         []string
	tagCount         map[string]uint64
	taskWithTagCount map[string]uint64
}

// NewTagCountTracer creates a new TagCountTracer
func NewTagCountTracer(filter TaskFilter) *TagCountTracer {
	return &TagCountTracer{
		filter:           filter,
		inflightTasks:    make(map[uint64]map[string]bool),
		tagCount:         make(map[string]uint64),
		taskWithTagCount: make(map[string]uint64),
	}
}

// TagNames returns all the tag names collected. It returns a copy so the
// caller cannot mutate, or race against appends to, the tracer's internal
// slice.
func (t *TagCountTracer) TagNames() []string {
	t.lock.Lock()
	defer t.lock.Unlock()

	names := make([]string, len(t.tagNames))
	copy(names, t.tagNames)

	return names
}

// TagCount returns the number of tags recorded with a certain tag name.
func (t *TagCountTracer) TagCount(tagName string) uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tagCount[tagName]
}

// TaskCount returns the number of tasks that carry at least one tag with the
// given name.
func (t *TagCountTracer) TaskCount(tagName string) uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.taskWithTagCount[tagName]
}

// TaskStarted begins tracking a task's tags.
func (t *TagCountTracer) TaskStarted(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.filter != nil && !t.filter(task) {
		return
	}

	t.inflightTasks[task.ID] = make(map[string]bool)
}

// TagAdded counts the tag and the task that carries it.
func (t *TagCountTracer) TagAdded(tag tasks.Tag) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if _, ok := t.tagCount[tag.What]; !ok {
		t.tagNames = append(t.tagNames, tag.What)
	}

	t.tagCount[tag.What]++

	seen, ok := t.inflightTasks[tag.TaskID]
	if !ok {
		return
	}

	if !seen[tag.What] {
		t.taskWithTagCount[tag.What]++
		seen[tag.What] = true
	}
}

// TaskEnded stops tracking the task.
func (t *TagCountTracer) TaskEnded(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	delete(t.inflightTasks, task.ID)
}
