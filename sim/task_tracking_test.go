package sim_test

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
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
				require.Nil(t, s.Tasks())
				return
			}
			require.NotNil(t, s.Tasks())
			s.Tasks().Subscribe(tasks.NopObserver{})
			require.NoError(t, s.Initialize())
			require.NoError(t, s.Engine().Run())
			require.Panics(t, func() { s.Tasks().Subscribe(tasks.NopObserver{}) })
		})
	}
}
