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
			b := sim.MakeBuilder().WithoutSourceRecording().WithOutputPath(filepath.Join(t.TempDir(), "trace") + ".sqlite3")
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
	s := sim.MakeBuilder().WithOutputPath(filepath.Join(t.TempDir(), "disabled") + ".sqlite3").Build()
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
		WithOutputPath(filepath.Join(t.TempDir(), "manual") + ".sqlite3").Build()
	defer s.Terminate()
	require.NotNil(t, s.VisTracer())
	require.False(t, s.VisTracer().IsTracing())
	require.NoError(t, s.Initialize())

	s.VisTracer().StartTracing()
	owner := &taskRecordingOwner{Simulation: s}
	advanceTaskClock(t, s, 10)
	id := owner.TaskTracker().StartTask(owner, tasks.StartInfo{Kind: tasks.KindReqIn, What: "manual"})
	require.NotZero(t, id)
	advanceTaskClock(t, s, 20)
	owner.TaskTracker().EndTask(id)
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
	t.Logf("WithTaskTracking and WithOutputPath: StartTracing recorded task %d through the tracker at %s over [%v,%v]",
		id, location, start, end)
}

type taskRecordingOwner struct {
	*sim.Simulation
}

func (*taskRecordingOwner) Name() string { return "Agent" }

type taskClockHandler struct{}

func (taskClockHandler) Handle(timing.Event) {}
func advanceTaskClock(t *testing.T, s *sim.Simulation, at timing.VTimeInPicoSec) {
	t.Helper()
	if at == 10 {
		s.Engine().RegisterHandler("Clock", taskClockHandler{})
	}
	s.Engine().Schedule(timing.MakeEventBase(s.NewID(), at, "Clock"))
	require.NoError(t, s.Run())
}

type namedTaskOwner struct{ name string }

func (o *namedTaskOwner) Name() string { return o.name }

type taskTimes struct {
	tasks.NopObserver
	ended tasks.Task
}

func (r *taskTimes) TaskEnded(task tasks.Task) { r.ended = task }

func TestNamedOwnerUsesSimulationServicesAndScopeIdentity(t *testing.T) {
	s := sim.MakeBuilder().WithTaskTracking().Build()
	foreign := sim.MakeBuilder().WithTaskTracking().Build()
	localOwner, foreignOwner := &namedTaskOwner{"Worker"}, &namedTaskOwner{"Worker"}
	s.RegisterResource(localOwner)
	foreign.RegisterResource(foreignOwner)
	observer := &taskTimes{}
	tracker := s.TaskTracker()
	tracker.Subscribe(observer, localOwner)
	require.Panics(t, func() { tracker.Subscribe(observer, foreignOwner) })
	require.Panics(t, func() { tracker.Subscribe(observer, &namedTaskOwner{"Worker"}) })
	require.NoError(t, s.Initialize())
	advanceTaskClock(t, s, 10)
	prior := s.NewID()
	id := tracker.StartTask(localOwner, tasks.StartInfo{Kind: "work", What: "named-only"})
	require.Equal(t, prior+1, id)
	advanceTaskClock(t, s, 20)
	tracker.EndTask(id)
	require.Equal(t, timing.VTimeInPicoSec(10), observer.ended.StartTime)
	require.Equal(t, timing.VTimeInPicoSec(20), observer.ended.EndTime)
	require.NoError(t, s.Terminate())
	require.NoError(t, foreign.Terminate())
}
