package simulation

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/stretchr/testify/require"
)

func buildIDTestSimulation(t *testing.T, parallel bool) *Simulation {
	t.Helper()
	b := MakeBuilder().WithoutMonitoring().WithoutSourceRecording().
		WithOutputFileName(filepath.Join(t.TempDir(), "simulation"))
	if parallel {
		b = b.WithParallelEngine()
	}
	s := b.Build()
	t.Cleanup(s.Terminate)
	return s
}

type allocatingHandler struct {
	ids       timing.IDSource
	allocated uint64
}

func (h *allocatingHandler) Handle(timing.Event) { h.allocated = h.ids.NewID() }

func TestSimulationsAllocateIndependentlyWhileRunning(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		name := "serial"
		if parallel {
			name = "parallel"
		}
		t.Run(name, func(t *testing.T) {
			sims := []*Simulation{buildIDTestSimulation(t, parallel), buildIDTestSimulation(t, parallel)}
			handlers := make([]*allocatingHandler, 2)
			for i, s := range sims {
				comp := modeling.NewBuilder[modeling.None, modeling.None, modeling.None]().
					WithEngine(s.GetEngine()).WithFreq(timing.GHz).Build("Comp")
				require.Same(t, s.GetIDGenerator(), comp.GetIDGenerator())
				require.Equal(t, uint64(1), comp.NewID())
				h := &allocatingHandler{ids: comp}
				handlers[i] = h
				s.GetEngine().(timing.HandlerRegistrar).RegisterHandler("handler", h)
				e := timing.MakeEventBase(s, 1, "handler")
				require.Equal(t, uint64(2), e.ID)
				s.GetEngine().Schedule(e)
			}
			var wg sync.WaitGroup
			errs := make([]error, len(sims))
			for i, s := range sims {
				wg.Add(1)
				go func() { defer wg.Done(); errs[i] = s.GetEngine().Run() }()
			}
			wg.Wait()
			for i, s := range sims {
				require.NoError(t, errs[i])
				require.Equal(t, uint64(3), handlers[i].allocated)
				require.Equal(t, uint64(4), s.NewID())
			}
		})
	}
}

func TestRestoringSimulationDoesNotChangeOtherIDCounters(t *testing.T) {
	a, b := buildIDTestSimulation(t, false), buildIDTestSimulation(t, false)
	for range 10 {
		a.NewID()
	}
	for range 100 {
		b.NewID()
	}
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	require.NoError(t, a.SaveCheckpoint(path, "ids"))
	require.Equal(t, uint64(11), a.NewID())
	require.Equal(t, uint64(101), b.NewID())
	restored := buildIDTestSimulation(t, false)
	require.NoError(t, restored.LoadCheckpoint(path, "ids"))
	require.NotSame(t, a.GetIDGenerator(), restored.GetIDGenerator())
	require.Equal(t, uint64(11), restored.NewID())
	require.Equal(t, uint64(12), a.NewID())
	require.Equal(t, uint64(102), b.NewID())
}
