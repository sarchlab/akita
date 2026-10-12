package sim

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckpointAutomaticBuildIdentity(t *testing.T) {
	build := func(t *testing.T, label string) *Simulation {
		t.Helper()
		s := MakeBuilder().
			WithOutputFileName(filepath.Join(t.TempDir(), label)).Build()
		t.Cleanup(s.Terminate)
		require.NoError(t, s.Initialize())
		return s
	}

	t.Run("automatic identity round trip", func(t *testing.T) {
		src, dst := build(t, "src"), build(t, "dst")
		path := filepath.Join(t.TempDir(), "default.tar.gz")
		src.NewID()
		require.NoError(t, src.SaveCheckpoint(path))
		savedID, _, err := readArchive(path)
		require.NoError(t, err)
		require.NotEmpty(t, savedID)
		require.Equal(t, defaultBuildID(), savedID)
		require.NoError(t, dst.LoadCheckpoint(path))
		require.Equal(t, src.NewID(), dst.NewID())
	})

	t.Run("different build identity mismatch", func(t *testing.T) {
		src, dst := build(t, "src"), build(t, "dst")
		// Simulate different binaries without exposing a public override.
		src.buildID, dst.buildID = "b", "a"
		path := filepath.Join(t.TempDir(), "different-build.tar.gz")
		require.NoError(t, src.SaveCheckpoint(path))
		require.EqualError(t, dst.LoadCheckpoint(path),
			`checkpoint: build ID mismatch: checkpoint "b", current "a"`)
	})
}
