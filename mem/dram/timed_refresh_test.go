package dram

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestTimedRefreshDuringIdleAndRestore(t *testing.T) {
	spec := DDR4Spec()
	spec.TREFI, spec.TRFC, spec.Freq = 20, 5, timing.GHz
	build := func() (*sim.Simulation, *Comp, *timing.SerialEngine) {
		s := sim.MakeBuilder().Build()
		c := buildDRAM(s, spec, "DRAM", 4)
		require.NoError(t, s.Initialize())
		return s, c, s.Engine().(*timing.SerialEngine)
	}
	s, c, e := build()
	require.NoError(t, s.Run())
	require.Zero(t, e.CurrentTime())
	c.TickAt(61000)
	require.NoError(t, e.RunUntil(21000))
	require.True(t, c.State.RefreshInProgress)
	require.EqualValues(t, 20000, e.CurrentTime())
	path := filepath.Join(t.TempDir(), "checkpoint.zip")
	require.NoError(t, s.SaveCheckpoint(path))
	s, c, e = build()
	require.NoError(t, s.LoadCheckpoint(path))
	require.NoError(t, e.RunUntil(26000))
	require.False(t, c.State.RefreshInProgress)
	require.EqualValues(t, 25000, e.CurrentTime())
	require.EqualValues(t, 40000, c.State.RefreshAt)
	require.NoError(t, s.Run())
	require.True(t, c.State.RefreshInProgress)
	require.EqualValues(t, 61000, e.CurrentTime())
	require.EqualValues(t, 65000, c.State.RefreshAt)
}

func TestTimedRefreshCanBeDisabled(t *testing.T) {
	spec := DDR4Spec()
	for _, timing := range []struct{ interval, duration int }{{0, 5}, {20, 0}} {
		spec.TREFI, spec.TRFC = timing.interval, timing.duration
		h := newP0Harness(t, spec)
		h.dram.TickAt(100000)
		require.NoError(t, h.engine.Run())
		require.Zero(t, h.dram.State.RefreshAt)
		require.False(t, h.dram.State.RefreshInProgress)
	}
}

func TestPauseFreezesTimedRefresh(t *testing.T) {
	spec := DDR4Spec()
	spec.TREFI, spec.TRFC, spec.Freq = 20, 5, timing.GHz
	h := newP0Harness(t, spec)
	c := h.dram
	c.TickAt(8000)
	require.NoError(t, h.engine.Run())
	c.Middlewares.Ctrl.pause()
	c.TickAt(31000)
	require.NoError(t, h.engine.Run())
	require.False(t, c.State.RefreshInProgress)
	c.Middlewares.Ctrl.resumeTimer()
	c.State.ControlState = controlStateEnabled
	require.EqualValues(t, 43000, c.State.RefreshAt)
	c.TickAt(44000)
	require.NoError(t, h.engine.Run())
	require.True(t, c.State.RefreshInProgress)
	require.EqualValues(t, 48000, c.State.RefreshAt)
}

func TestWaitingRequestCountsRefreshStallCycles(t *testing.T) {
	spec := DDR4Spec()
	spec.TREFI, spec.TRFC, spec.Freq = 20, 15, timing.GHz
	h := newP0Harness(t, spec)
	e := h.engine.(*timing.SerialEngine)
	h.dram.TickAt(21000)
	require.NoError(t, e.RunUntil(20000))
	h.src.Send(h.read(0))
	require.NoError(t, e.RunUntil(27000))
	require.True(t, h.dram.State.RefreshBlockedIssue)
	start := h.dram.State.TickCount
	require.NoError(t, e.RunUntil(32000))
	require.EqualValues(t, 32000, e.CurrentTime())
	require.Equal(t, start+5, h.dram.State.TickCount)
	require.NoError(t, e.Run())
	reads, _ := h.collect()
	require.Len(t, reads, 1)
}
