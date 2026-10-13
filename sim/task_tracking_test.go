package sim_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestTrackingEnablementAndRunBoundary(t *testing.T) {
	for _, mode := range []string{"disabled", "explicit", "recording", "monitor"} {
		t.Run(mode, func(t *testing.T) {
			b := sim.MakeBuilder().WithoutSourceRecording().WithOutputFileName(filepath.Join(t.TempDir(), "trace"))
			switch mode {
			case "explicit":
				b = b.WithTaskTracking()
			case "recording":
				b = b.WithVisTracingOnStart()
			case "monitor":
				b = b.WithMonitor(&lifecycleMonitor{})
			}
			s := b.Build()
			defer s.Terminate()
			if mode == "disabled" {
				require.Nil(t, s.TaskTracker())
				require.Nil(t, s.VisTracer())
				return
			}
			require.NotNil(t, s.TaskTracker())
			require.NotNil(t, s.VisTracer())
			s.TaskTracker().Subscribe(tasks.NopObserver{})
			require.NoError(t, s.Initialize())
			require.NoError(t, s.Engine().Run())
			require.NotPanics(t, func() { s.TaskTracker().Subscribe(tasks.NopObserver{}) })
		})
	}
}

func TestDisabledTrackingCreatesNoTraceTables(t *testing.T) {
	s := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "disabled")).Build()
	s.Terminate()

	db, err := sql.Open("sqlite", s.TraceDBPath())
	require.NoError(t, err)
	defer db.Close()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('trace', 'milestone', 'tag', 'daisen$segments')`).Scan(&count))
	require.Zero(t, count)
	t.Log("Tracking disabled: Terminate succeeded and SQLite contains no task, milestone, tag, or segment tables")
}

func TestExplicitTrackingSupportsManualRecording(t *testing.T) {
	s := sim.MakeBuilder().WithTaskTracking().
		WithOutputFileName(filepath.Join(t.TempDir(), "manual")).Build()
	defer s.Terminate()
	require.Nil(t, s.Monitor())
	require.NotNil(t, s.VisTracer())
	require.False(t, s.VisTracer().IsTracing())
	require.NoError(t, s.Initialize())

	s.VisTracer().StartTracing()
	owner := &taskRecordingOwner{Simulation: s, now: 10}
	id := tasks.Start(owner, tasks.StartInfo{Kind: tasks.KindReqIn, What: "manual"})
	require.NotZero(t, id)
	owner.now = 20
	tasks.End(owner, tasks.EndInfo{ID: id})
	s.VisTracer().StopTracing()
	s.DataRecorder().Flush()

	db, err := sql.Open("sqlite", s.TraceDBPath())
	require.NoError(t, err)
	defer db.Close()
	var location string
	var start, end float64
	require.NoError(t, db.QueryRow(`SELECT l.Locale, t.StartTime, t.EndTime
		FROM trace t JOIN location l ON t.Location = l.ID
		WHERE t.ID = ? AND t.What = 'manual'`, id).Scan(&location, &start, &end))
	require.Equal(t, "Agent.req_in", location)
	require.Equal(t, float64(10), start)
	require.Equal(t, float64(20), end)
	t.Logf("WithTaskTracking alone: StartTracing recorded task %d through the tracker at %s over [%v,%v]",
		id, location, start, end)
}

type taskRecordingOwner struct {
	*sim.Simulation
	now timing.VTimeInPicoSec
}

func (*taskRecordingOwner) Name() string                         { return "Agent" }
func (o *taskRecordingOwner) CurrentTime() timing.VTimeInPicoSec { return o.now }
