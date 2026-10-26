package httpapi

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/sarchlab/akita/v5/internal/sourcefs"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/stretchr/testify/require"
)

// recordingAgent is a minimal port-owning component for the round trip.
type recordingAgent struct{ name string }

func (a *recordingAgent) Name() string                           { return a.name }
func (a *recordingAgent) BindPort(slot string, p messaging.Port) { p.BindOwner(a, a.name+"."+slot) }
func (a *recordingAgent) NotifyRecv(messaging.Port)              {}
func (a *recordingAgent) NotifyPortFree(messaging.Port)          {}

// TestReaderReadsSimulationRecording writes a real traced simulation, whose
// application also records its own trace and location tables, and checks that
// Daisen reads Akita's tables back: the task with its tag and milestone, the
// tracing segment, execution info, topology, and recorded source. The other
// reader tests create the tables by hand, so only this test catches a writer
// and reader that disagree on a table name.
func TestReaderReadsSimulationRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.sqlite3")
	s := sim.MakeBuilder().WithOutputPath(path).WithVisTracingOnStart().
		WithSourceFS("example.com/app", fstest.MapFS{"main.go": {Data: []byte("package main\n")}}).
		Build()

	type appRow struct{ Name string }
	s.DataRecorder().CreateTable("trace", appRow{})
	s.DataRecorder().CreateTable("location", appRow{})
	s.DataRecorder().InsertData("trace", appRow{Name: "app row"})

	agent := &recordingAgent{name: "Agent"}
	s.RegisterComponent(agent)
	port := twowaybuffered.MakeBuilder().WithSimulation(s).Build()
	agent.BindPort("Top", port)
	s.RegisterPort(port)

	tracker := s.TaskTracker()
	id := tracker.StartTask(agent, tasks.StartInfo{Kind: tasks.KindReqIn, What: "ReadReq"})
	tracker.AddTag(tasks.Tag{TaskID: id, What: "read-hit"})
	tracker.AddMilestone(tasks.Milestone{TaskID: id, Kind: tasks.MilestoneKindData, What: "arrived"})
	tracker.EndTask(id)
	require.NoError(t, s.Terminate())

	reader := NewSQLiteTraceReader(path)
	reader.Init()
	t.Cleanup(func() { _ = reader.Close() })
	ctx := context.Background()

	got := reader.ListTasks(ctx, TaskQuery{ID: id, EnableMilestones: true})
	require.Len(t, got, 1)
	require.Equal(t, "Agent.req_in", got[0].Location)
	require.Equal(t, "ReadReq", got[0].What)
	steps := make([]string, 0, len(got[0].Steps))
	for _, step := range got[0].Steps {
		steps = append(steps, step.What)
	}
	require.ElementsMatch(t, []string{"read-hit", "arrived"}, steps)

	segments := reader.ListSegments(ctx)
	require.True(t, segments.Enabled)
	require.Len(t, segments.Segments, 1)

	info := reader.ListExecInfo(ctx)
	properties := make([]string, 0, len(info))
	for _, entry := range info {
		properties = append(properties, entry.Property)
	}
	require.Contains(t, properties, "Command")
	require.Contains(t, properties, "End Time")

	topology := reader.ReadTopology(ctx)
	require.Len(t, topology.Components, 1)
	require.Equal(t, "Agent", topology.Components[0].Name)
	require.Equal(t, []TopologyPort{{Component: "Agent", Port: "Agent.Top"}}, topology.Ports)

	source, err := sourcefs.OpenTraceSource(reader.DB)
	require.NoError(t, err)
	require.Contains(t, source.Roots, "example.com/app")

	var appName string
	require.NoError(t, reader.QueryRow("SELECT Name FROM trace").Scan(&appName))
	require.Equal(t, "app row", appName)
	t.Logf("Daisen read task %d at %s with steps %v, %d segment, %d akita$exec_info rows, "+
		"topology %+v, and source roots %v beside the application's own trace table",
		id, got[0].Location, steps, len(segments.Segments), len(properties), topology.Ports, source.Roots)
}
