package timing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParallelEngineSchedulesEventsInOrder(t *testing.T) {
	engine := NewParallelEngine()

	recorder := &callRecorder{t: t}

	handlerA := &recordingHandler{name: "A", recorder: recorder}
	handlerB := &recordingHandler{name: "B", recorder: recorder, schedule: map[string][]ScheduledEvent{
		"evt2": {
			{Event: "evt3", Time: VTimeInCycle(3), Handler: handlerA},
			{Event: "evt4", Time: VTimeInCycle(5), Handler: handlerA},
		},
	}}

	handlerA.engine = engine
	handlerB.engine = engine

	engine.Schedule(ScheduledEvent{Event: "evt1", Time: VTimeInCycle(4), Handler: handlerA})
	engine.Schedule(ScheduledEvent{Event: "evt2", Time: VTimeInCycle(2), Handler: handlerB})

	require.NoError(t, engine.Run())

	recorder.assertOrder(t, []string{"B:evt2", "A:evt3", "A:evt1", "A:evt4"})
}

func TestParallelEngineProcessesConcurrentEvents(t *testing.T) {
	engine := NewParallelEngine()

	recorder := &callRecorder{t: t}

	handlerPrimary1 := &recordingHandler{name: "P1", recorder: recorder}
	handlerPrimary2 := &recordingHandler{name: "P2", recorder: recorder}
	handlerSecondary := &recordingHandler{name: "S", recorder: recorder}

	handlerPrimary1.engine = engine
	handlerPrimary2.engine = engine
	handlerSecondary.engine = engine

	engine.Schedule(ScheduledEvent{Event: "secondary", Time: VTimeInCycle(2), Handler: handlerSecondary})
	engine.Schedule(ScheduledEvent{Event: "primary1", Time: VTimeInCycle(2), Handler: handlerPrimary1})
	engine.Schedule(ScheduledEvent{Event: "primary2", Time: VTimeInCycle(2), Handler: handlerPrimary2})

	require.NoError(t, engine.Run())

	calls := recorder.snapshot()
	require.Equal(t, 3, len(calls))
	require.ElementsMatch(t,
		[]string{"P1:primary1", "P2:primary2", "S:secondary"},
		calls,
	)
}
