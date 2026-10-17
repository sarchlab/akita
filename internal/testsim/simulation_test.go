package testsim

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type probeHandler struct{ count int }

func (h *probeHandler) Handle(timing.Event) { h.count++ }
func TestRunAndCheckpointUseInjectedEngine(t *testing.T) {
	engine := timing.NewSerialEngine()
	s := New(engine, true)
	handler := &probeHandler{}
	require.Same(t, engine, s.Engine())
	engine.RegisterHandler("probe", handler)
	engine.Schedule(timing.EventBase{At: 42, Handler: "probe"})
	require.Error(t, s.Run())
	require.Zero(t, handler.count)
	require.NoError(t, s.Initialize())
	path := filepath.Join(t.TempDir(), "checkpoint.zip")
	require.NoError(t, s.SaveCheckpoint(path))
	require.NoError(t, s.Run())
	require.Equal(t, 1, handler.count)
	require.Equal(t, timing.VTimeInPicoSec(42), engine.CurrentTime())
	require.NoError(t, s.LoadCheckpoint(path))
	require.NoError(t, s.Run())
	require.Equal(t, 2, handler.count)
	t.Log("Run rejected execution before initialization, then executed the injected event exactly once; " +
		"restoring the checkpoint restored that same engine's queued event")
}
func TestNonSerialEngineCannotCheckpoint(t *testing.T) {
	s := New(timing.NewParallelEngine(), false)
	require.NoError(t, s.Initialize())
	require.Error(t, s.SaveCheckpoint(filepath.Join(t.TempDir(), "checkpoint.zip")))
}
