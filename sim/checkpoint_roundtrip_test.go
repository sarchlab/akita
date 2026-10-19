package sim

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckpointEntityOnlyRoundTrip(t *testing.T) {
	src, dst := MakeBuilder().Build(), MakeBuilder().Build()
	t.Cleanup(func() { require.NoError(t, src.Terminate()); require.NoError(t, dst.Terminate()) })
	require.NoError(t, src.Initialize())
	require.NoError(t, dst.Initialize())
	src.NewID()
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	require.NoError(t, src.SaveCheckpoint(path))
	require.NotContains(t, archiveEntryNames(t, path), "build_id")
	require.NoError(t, dst.LoadCheckpoint(path))
	require.Equal(t, src.NewID(), dst.NewID())
}
