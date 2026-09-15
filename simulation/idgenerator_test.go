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
	ids       timing.Simulation
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
					WithSimulation(s).WithFreq(timing.GHz).Build("Comp")
				require.Same(t, s, comp.Simulation())
				require.Equal(t, uint64(1), comp.Simulation().NewID())
				h := &allocatingHandler{ids: comp.Simulation()}
				handlers[i] = h
				s.GetEngine().(timing.HandlerRegistry).RegisterHandler("handler", h)
				e := timing.MakeEventBase(s.NewID(), 1, "handler")
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

func TestConcurrentSimulationIDAllocation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		parallel bool
	}{
		{"serial", false},
		{"parallel", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := buildIDTestSimulation(t, tc.parallel), buildIDTestSimulation(t, tc.parallel)
			require.NotSame(t, a.GetIDGenerator(), b.GetIDGenerator())
			const workers, perWorker = 8, 1024
			var wg sync.WaitGroup
			results := make([][]uint64, workers)
			for worker := range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results[worker] = make([]uint64, perWorker)
					for i := range perWorker {
						results[worker][i] = a.NewID()
					}
				}()
			}
			wg.Wait()
			seen := make(map[uint64]bool)
			for _, batch := range results {
				for _, id := range batch {
					require.NotZero(t, id)
					require.False(t, seen[id], "duplicate ID %d", id)
					seen[id] = true
				}
			}
			require.Len(t, seen, workers*perWorker)
			require.Equal(t, uint64(workers*perWorker+1), a.NewID())
			require.Equal(t, uint64(1), b.NewID())
			require.Equal(t, uint64(2), b.NewID())
			t.Logf("%s: %d unique IDs from %d concurrent callers; second simulation starts at 1", tc.name, len(seen), workers)
		})
	}
}

func TestComponentsScheduleWithSharedSimulationIDs(t *testing.T) {
	s := buildIDTestSimulation(t, false)
	ticked := modeling.NewBuilder[modeling.None, modeling.None, modeling.None]().
		WithSimulation(s).WithFreq(timing.GHz).Build("Ticked")
	eventDriven := modeling.NewEventDrivenBuilder[modeling.None, modeling.None, modeling.None]().
		WithSimulation(s).Build("EventDriven")
	require.Same(t, s, ticked.Simulation())
	require.Same(t, s, eventDriven.Simulation())
	ticked.TickLater()
	eventDriven.ScheduleWakeAt(1000)
	require.Equal(t, uint64(3), s.NewID(), "both scheduled events must use the simulation's counter")
}
