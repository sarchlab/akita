package tracing

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rs/xid"
	"github.com/tebeka/atexit"
)

// JSONTracer can write tasks into json format.
type JSONTracer struct {
	w             io.Writer
	lock          sync.Mutex
	firstTask     bool
	inflightTasks map[string]Task
}

// StartTask records the start of a task
func (t *JSONTracer) StartTask(task Task) {
	t.lock.Lock()
	t.inflightTasks[task.ID] = task
	t.lock.Unlock()
}

// StepTask records the moment that a task reaches a milestone
func (t *JSONTracer) StepTask(task Task) {
	// Do nothing right now
}

// EndTask records the time that a task is completed.
func (t *JSONTracer) EndTask(task Task) {
	t.lock.Lock()
	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		t.lock.Unlock()
		return
	}
	originalTask.EndTime = task.EndTime

	delete(t.inflightTasks, task.ID)
	t.lock.Unlock()

	if t.firstTask {
		t.firstTask = false
	} else {
		_, err := t.w.Write([]byte(",\n"))
		if err != nil {
			panic(err)
		}
	}

	b, err := json.Marshal(originalTask)
	if err != nil {
		panic(err)
	}

	_, err = t.w.Write(b)
	if err != nil {
		panic(err)
	}
}

func (t *JSONTracer) finish() {
	_, err := t.w.Write([]byte("\n]"))
	if err != nil {
		panic(err)
	}
}

// NewJSONTracer creates a new JsonTracer, injecting a writer
// as dependency.
func NewJSONTracer() *JSONTracer {
	filename := xid.New().String() + ".json"
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Recodring tasks in %s\n", filename)

	_, err = f.Write([]byte("[\n"))
	if err != nil {
		panic(err)
	}

	t := &JSONTracer{
		w:             f,
		firstTask:     true,
		inflightTasks: make(map[string]Task),
	}

	atexit.Register(t.finish)

	return t
}
