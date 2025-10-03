package timing

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// scheduler captures the subset of engine behaviour recordingHandler relies on.
type scheduler interface {
	Schedule(FutureEvent)
	CurrentTime() VTimeInCycle
}

type recordingHandler struct {
	name     string
	engine   scheduler
	recorder *callRecorder
	schedule map[string][]FutureEvent
}

func (h *recordingHandler) Handle(event any) error {
	label, _ := event.(string)
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
	handlerB := &recordingHandler{name: "B", recorder: recorder, schedule: map[string][]FutureEvent{
		"evt2": {
			{Event: "evt3", Time: VTimeInCycle(3), Handler: handlerA},
			{Event: "evt4", Time: VTimeInCycle(5), Handler: handlerA},
		},
	}}

	handlerA.engine = engine
	handlerB.engine = engine

	engine.Schedule(FutureEvent{Event: "evt1", Time: VTimeInCycle(4), Handler: handlerA})
	engine.Schedule(FutureEvent{Event: "evt2", Time: VTimeInCycle(2), Handler: handlerB})

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

	engine.Schedule(FutureEvent{Event: "secondary", Time: VTimeInCycle(2), Handler: handlerSecondary})
	engine.Schedule(FutureEvent{Event: "primary1", Time: VTimeInCycle(2), Handler: handlerPrimary1})
	engine.Schedule(FutureEvent{Event: "primary2", Time: VTimeInCycle(2), Handler: handlerPrimary2})

	require.NoError(t, engine.Run())

	calls := recorder.snapshot()
	require.Equal(t, 3, len(calls))
	require.ElementsMatch(t,
		[]string{"P1:primary1", "P2:primary2", "S:secondary"},
		calls,
	)
}
