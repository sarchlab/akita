package timing

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// scheduler captures the subset of engine behaviour recordingHandler relies on.
type scheduler interface {
	Schedule(Event)
	CurrentTime() VTimeInCycle
}

type scheduledStringEvent struct {
	cycle  VTimeInCycle
	target Handler
	label  string
}

func newStringEvent(label string, time VTimeInCycle, handler Handler) Event {
	return &scheduledStringEvent{cycle: time, target: handler, label: label}
}

func (e *scheduledStringEvent) Time() VTimeInCycle { return e.cycle }

func (e *scheduledStringEvent) Handler() Handler { return e.target }

type recordingHandler struct {
	name     string
	engine   scheduler
	recorder *callRecorder
	schedule map[string][]Event
}

func (h *recordingHandler) Handle(event any) error {
	evt, ok := event.(*scheduledStringEvent)
	if !ok {
		return fmt.Errorf("unexpected event type: %T", event)
	}

	label := evt.label
	h.recorder.record(h.name + ":" + label)

	if events, ok := h.schedule[label]; ok {
		for _, evt := range events {
			h.engine.Schedule(evt)
		}
	}

	return nil
}

type callRecorder struct {
	t     *testing.T
	mu    sync.Mutex
	calls []string
}

func (r *callRecorder) record(entry string) {
	r.mu.Lock()
	r.calls = append(r.calls, entry)
	r.mu.Unlock()
}

func (r *callRecorder) assertOrder(t *testing.T, expected []string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Equal(t, expected, r.calls)
}

func (r *callRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	dup := make([]string, len(r.calls))
	copy(dup, r.calls)
	return dup
}

func TestSerialEngineSchedulesEventsInOrder(t *testing.T) {
	engine := NewSerialEngine()

	recorder := &callRecorder{t: t}

	handlerA := &recordingHandler{name: "A", recorder: recorder}
	handlerB := &recordingHandler{name: "B", recorder: recorder, schedule: map[string][]Event{
		"evt2": {
			newStringEvent("evt3", VTimeInCycle(3), handlerA),
			newStringEvent("evt4", VTimeInCycle(5), handlerA),
		},
	}}

	handlerA.engine = engine
	handlerB.engine = engine

	engine.Schedule(newStringEvent("evt1", VTimeInCycle(4), handlerA))
	engine.Schedule(newStringEvent("evt2", VTimeInCycle(2), handlerB))

	require.NoError(t, engine.Run())

	recorder.assertOrder(t, []string{"B:evt2", "A:evt3", "A:evt1", "A:evt4"})
}

func TestSerialEngineProcessesConcurrentEvents(t *testing.T) {
	engine := NewSerialEngine()

	recorder := &callRecorder{t: t}

	handlerPrimary1 := &recordingHandler{name: "P1", recorder: recorder}
	handlerPrimary2 := &recordingHandler{name: "P2", recorder: recorder}
	handlerSecondary := &recordingHandler{name: "S", recorder: recorder}

	handlerPrimary1.engine = engine
	handlerPrimary2.engine = engine
	handlerSecondary.engine = engine

	engine.Schedule(newStringEvent("secondary", VTimeInCycle(2), handlerSecondary))
	engine.Schedule(newStringEvent("primary1", VTimeInCycle(2), handlerPrimary1))
	engine.Schedule(newStringEvent("primary2", VTimeInCycle(2), handlerPrimary2))

	require.NoError(t, engine.Run())

	calls := recorder.snapshot()
	require.Equal(t, 3, len(calls))
	require.ElementsMatch(t,
		[]string{"P1:primary1", "P2:primary2", "S:secondary"},
		calls,
	)
}
