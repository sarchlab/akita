package timing

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDuplicateHandlerKeepsOriginal(t *testing.T) {
	for _, e := range []Engine{NewSerialEngine(), NewParallelEngine()} {
		t.Run(e.(interface{ Name() string }).Name(), func(t *testing.T) {
			handled := 0
			original := panicHandler(func(Event) { handled++ })
			e.RegisterHandler("handler", original)
			require.PanicsWithValue(t, `timing: duplicate handler name "handler"`,
				func() { e.RegisterHandler("handler", original) })
			require.Panics(t, func() {
				e.RegisterHandler("handler", panicHandler(func(Event) { t.Error("replaced original") }))
			})
			e.Schedule(MakeEventBase(1, 1, "handler"))
			require.NoError(t, e.Run())
			require.Equal(t, 1, handled)
		})
	}
}

type unregisteredCheckpointEvent struct{ EventBase }

func TestCheckpointRejectsUnregisteredEventBeforeWriting(t *testing.T) {
	e := NewSerialEngine()
	e.RegisterHandler("h", panicHandler(func(Event) {}))
	evt := unregisteredCheckpointEvent{MakeEventBase(1, 1, "h")}
	require.NotPanics(t, func() { e.Schedule(evt) })
	var data bytes.Buffer
	err := e.SaveCheckpoint(&data)
	require.ErrorContains(t, err, "unregistered event type timing.unregisteredCheckpointEvent")
	require.ErrorContains(t, err, "register it before checkpointing")
	require.Empty(t, data.Bytes())
}
