package tracers

import (
	"sync"

	"github.com/sarchlab/akita/v4/instrumentation/tracing"
)

// TagCountTracer can collect how often a certain tag is triggered.
type TagCountTracer struct {
	filter           tracing.TaskFilter
	lock             sync.Mutex
	inflightTasks    map[string]tracing.Task
	tagNames         []string
	tagCount         map[string]uint64
	taskWithTagCount map[string]uint64
}

// NewTagCountTracer creates a new TagCountTracer.
func NewTagCountTracer(filter tracing.TaskFilter) *TagCountTracer {
	t := &TagCountTracer{
		filter:           filter,
		inflightTasks:    make(map[string]tracing.Task),
		tagCount:         make(map[string]uint64),
		taskWithTagCount: make(map[string]uint64),
	}

	return t
}

// GetTagNames returns all the tag names collected.
func (t *TagCountTracer) GetTagNames() []string {
	return t.tagNames
}

// GetTagCount returns the number of tags that are recorded with a certain tag
// name.
func (t *TagCountTracer) GetTagCount(tagName string) uint64 {
	return t.tagCount[tagName]
}

// GetTaskCount returns the number of tasks that are recorded to have a certain
// tag with a given name.
func (t *TagCountTracer) GetTaskCount(tagName string) uint64 {
	return t.taskWithTagCount[tagName]
}

// StartTask records the task start time
func (t *TagCountTracer) StartTask(task tracing.Task) {
	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = task
	t.lock.Unlock()
}

// TagTask counts the provided tag occurrence.
func (t *TagCountTracer) TagTask(task tracing.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.countTag(task)
	t.countTask(task)
}

func (t *TagCountTracer) countTag(task tracing.Task) {
	tag := task.Tags[0]

	_, ok := t.tagCount[tag.What]
	if !ok {
		t.tagNames = append(t.tagNames, tag.What)
	}

	t.tagCount[tag.What]++
}

func (t *TagCountTracer) countTask(task tracing.Task) {
	tag := task.Tags[0]

	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	if !taskContainsTag(originalTask, tag) {
		t.taskWithTagCount[tag.What]++
	}

	originalTask.Tags = append(originalTask.Tags, tag)
}

func taskContainsTag(task tracing.Task, tag tracing.TaskTag) bool {
	for _, t := range task.Tags {
		if t.What == tag.What {
			return true
		}
	}

	return false
}

// TagCountTracer does nothing
func (t *TagCountTracer) AddMilestone(_ tracing.Milestone) {
	// Do nothing
}

// EndTask records the end of the task
func (t *TagCountTracer) EndTask(task tracing.Task) {
	t.lock.Lock()

	_, ok := t.inflightTasks[task.ID]
	if !ok {
		t.lock.Unlock()
		return
	}

	delete(t.inflightTasks, task.ID)
	t.lock.Unlock()
}
