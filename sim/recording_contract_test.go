package sim_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestDefaultAndTrackingOnlyCreateNoRecording(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, tracking := range []bool{false, true} {
		b := sim.MakeBuilder()
		if tracking {
			b = b.WithTaskTracking()
		}
		s := b.Build()
		require.Nil(t, s.DataRecorder())
		require.Nil(t, s.VisTracer())
		require.Empty(t, s.TraceDBPath())
		if tracking {
			require.NotNil(t, s.TaskTracker())
			s.TaskTracker().Subscribe(tasks.NopObserver{})
		}
		require.ErrorContains(t, s.Run(), "Initialize")
		// Direct engine execution deliberately does not enforce readiness.
		require.NoError(t, s.Engine().Run())
		require.NoError(t, s.Engine().(*timing.SerialEngine).RunUntil(1))
		require.NoError(t, s.Initialize())
		require.NoError(t, s.Run())
		s.Terminate()
	}
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
	t.Log("Default and tracking-only runs created no files; only Simulation.Run enforced readiness")
}

func TestRecordingUsesExactPathAndPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording #?.custom")
	s := sim.MakeBuilder().WithOutputPath(path).Build()
	require.Equal(t, path, s.TraceDBPath())
	require.NotNil(t, s.DataRecorder())
	type entry struct{ Value string }
	s.DataRecorder().CreateTable("literal_path", entry{})
	s.DataRecorder().InsertData("literal_path", entry{Value: "round trip"})
	s.Terminate()
	reader := datarecording.NewReader(path)
	reader.MapTable("literal_path", entry{})
	rows, count, err := reader.Query(context.Background(), "literal_path", datarecording.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "round trip", rows[0].(*entry).Value)
	require.NoError(t, reader.Close())
	require.FileExists(t, path)
	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NoFileExists(t, path+".sqlite3")
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Panics(t, func() { sim.MakeBuilder().WithOutputPath(path).Build() })
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, after)
	t.Log("Recording used the exact path, and a second Build rejected it without changing a byte")
}
