package timing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParallelEngineSchedulesEventsInOrder(t *testing.T) {
	engine := NewParallelEngine()

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

func TestParallelEngineProcessesConcurrentEvents(t *testing.T) {
	engine := NewParallelEngine()

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
