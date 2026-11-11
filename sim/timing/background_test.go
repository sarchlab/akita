package timing

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func backgroundTestEvent(at VTimeInPicoSec, id uint64, background, secondary bool) queueTestEvent {
	e := queueTestEvent{MakeEventBase(id, at, "h")}
	e.Background, e.Secondary = background, secondary
	return e
}

type backgroundFollowupHandler struct {
	engine Engine
	ids    []uint64
}

func (h *backgroundFollowupHandler) Handle(event Event) {
	id := event.(queueTestEvent).ID
	h.ids = append(h.ids, id)
	if id == 1 {
		h.engine.Schedule(backgroundTestEvent(7, 5, false, false))
	}
}

func TestRunWithBackgroundWork(t *testing.T) {
	for name, engine := range map[string]Engine{"serial": NewSerialEngine(), "parallel": NewParallelEngine()} {
		t.Run(name, func(t *testing.T) {
			rec := &backgroundFollowupHandler{engine: engine}
			engine.RegisterHandler("h", rec)
			engine.Schedule(backgroundTestEvent(2, 1, true, true))
			engine.Schedule(backgroundTestEvent(8, 2, true, false))
			require.NoError(t, engine.Run())
			require.Zero(t, engine.CurrentTime())
			engine.Schedule(backgroundTestEvent(5, 3, false, false))
			require.NoError(t, engine.Run())
			require.Equal(t, []uint64{1, 3, 5}, rec.ids)
			require.EqualValues(t, 7, engine.CurrentTime())
			engine.Schedule(backgroundTestEvent(9, 4, false, true))
			require.NoError(t, engine.Run())
			require.Equal(t, []uint64{1, 3, 5, 2, 4}, rec.ids)
		})
	}
}

func TestBackgroundCheckpointLoadPrecondition(t *testing.T) {
	RegisterEvent(queueTestEvent{})
	source := NewSerialEngine()
	source.RegisterHandler("h", &idRecordingHandler{})
	source.Schedule(backgroundTestEvent(9, 1, true, true))
	var checkpoint bytes.Buffer
	require.NoError(t, source.SaveCheckpoint(&checkpoint))
	engine := NewSerialEngine()
	rec := &idRecordingHandler{}
	engine.RegisterHandler("h", rec)
	engine.Schedule(backgroundTestEvent(1, 2, true, false))
	require.NoError(t, engine.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())))
	require.NoError(t, engine.RunUntil(100))
	require.Zero(t, engine.CurrentTime())
	engine.Schedule(backgroundTestEvent(0, 3, false, false))
	require.NoError(t, engine.Run())
	require.Error(t, engine.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())))
	engine.Schedule(backgroundTestEvent(10, 4, false, false))
	require.NoError(t, engine.RunUntil(8))
	require.Equal(t, []uint64{3}, rec.ids)
	require.NoError(t, engine.RunUntil(9))
	require.Equal(t, []uint64{3, 1}, rec.ids)
	require.NoError(t, engine.Run())
	require.Equal(t, []uint64{3, 1, 4}, rec.ids)
}
