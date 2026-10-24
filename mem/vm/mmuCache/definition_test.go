package mmuCache

import (
	"bytes"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/stretchr/testify/require"
)

// TestComponentMatchesSource asserts that the inspector's static view of
// this package equals the runtime component type and DefaultSpec() result, so the two can never drift.
func TestComponentMatchesSource(t *testing.T) {
	modelingtest.CheckTicking(t, (*Comp)(nil), DefaultSpec())
}

// TestBuildRequiresBlocks checks that Build rejects a cache with no blocks.
func TestBuildRequiresBlocks(t *testing.T) {
	spec := DefaultSpec()
	spec.NumBlocks = 0

	require.PanicsWithValue(t, "mmuCache: Spec.NumBlocks must be > 0", func() {
		setupSim1 := akitasim.MakeBuilder().Build()
		builtComponent := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(setupSim1).
			Build("MMUCache")
		builtComponentPorts := defaultPorts(setupSim1, "MMUCache")
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim1.Initialize(); err != nil {
			panic(err)
		}
	})
}

func TestCheckpointIncludesPeerAddresses(t *testing.T) {
	build := func(spec Spec) *Comp {
		s := akitasim.MakeBuilder().Build()
		t.Cleanup(func() { require.NoError(t, s.Terminate()) })
		c := modeling.MakeBuilder[*Comp](spec).WithSimulation(s).Build("MMUCache")
		ports := defaultPorts(s, "MMUCache")
		c.BindPort("Top", ports.Top)
		c.BindPort("Bottom", ports.Bottom)
		c.BindPort("Control", ports.Control)
		(&noopConn{}).BindPort(ports.Top)
		(&noopConn{}).BindPort(ports.Bottom)
		(&noopConn{}).BindPort(ports.Control)
		require.NoError(t, s.Initialize())
		return c
	}

	spec := DefaultSpec()
	spec.LowModule = "MMU.Top"
	spec.UpModule = "TLB.Bottom"
	var saved bytes.Buffer
	require.NoError(t, build(spec).SaveCheckpoint(&saved))
	require.NoError(t, build(spec).LoadCheckpoint(bytes.NewReader(saved.Bytes())))

	for _, peer := range []string{"LowModule", "UpModule"} {
		t.Run(peer, func(t *testing.T) {
			changed := spec
			if peer == "LowModule" {
				changed.LowModule = "OtherMMU.Top"
			} else {
				changed.UpModule = "OtherTLB.Bottom"
			}
			err := build(changed).LoadCheckpoint(bytes.NewReader(saved.Bytes()))
			require.ErrorContains(t, err, "spec hash mismatch")
			t.Logf("changing %s rejects the saved checkpoint", peer)
		})
	}
}
