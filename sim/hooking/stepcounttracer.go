package hooking

import (
	"sync"
)

// TagCountTracer can collect the total time of a certain step is triggered.
type TagCountTracer struct {
	filter     TaskFilter
	timeTeller TimeTeller
	lock       sync.Mutex

	inflightTasks map[string]task
	tagNames      []string
	tagCount      map[string]uint64
}

// NewTagCountTracer creates a new TagCountTracer
func NewTagCountTracer(
	filter TaskFilter,
	timeTeller TimeTeller,
) *TagCountTracer {
	t := &TagCountTracer{
		filter:        filter,
		timeTeller:    timeTeller,
		inflightTasks: make(map[string]task),
		tagCount:      make(map[string]uint64),
	}

	return t
}

// GetTagNames returns all the tag names collected.
func (t *TagCountTracer) GetTagNames() []string {
	return t.tagNames
}

// GetTagCount returns the number of steps that is recorded with a certain step
// name.
func (t *TagCountTracer) GetTagCount(tagName string) uint64 {
	return t.tagCount[tagName]
}

// TagTask tags a task with a certain tag.
func (t *TagCountTracer) TagTask(taskTag TaskTag) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.countTag(taskTag)
}

func (t *TagCountTracer) countTag(taskTag TaskTag) {
	_, ok := t.tagCount[taskTag.What]
	if !ok {
		t.tagNames = append(t.tagNames, taskTag.What)
	}

	t.tagCount[taskTag.What]++
}
