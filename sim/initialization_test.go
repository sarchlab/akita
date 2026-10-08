package sim

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type initializationPorts struct{ Top messaging.Port }
type initializationState struct {
	SawConnection bool `json:"saw_connection"`
}
type initializationComp = ticking.Component[
	tickCountSpec, initializationState, modeling.None, initializationPorts, modeling.None]

func TestExplicitBindingThenInitialization(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(map[bool]string{false: "serial", true: "parallel"}[parallel], func(t *testing.T) {
			builder := MakeBuilder().WithoutSourceRecording().WithOutputFileName(filepath.Join(t.TempDir(), "trace"))
			if parallel {
				builder = builder.WithParallelEngine()
			}
			s := builder.Build()
			t.Cleanup(s.Terminate)
			calls := 0
			def := ticking.Definition[tickCountSpec, initializationState, modeling.None, initializationPorts, modeling.None]{
				DefaultSpec: tickCountSpec{Freq: timing.GHz},
				NewState: func(c *initializationComp) initializationState {
					calls++
					return initializationState{SawConnection: c.Ports.Top.Connection() != nil}
				},
				NewMiddlewares: func(*initializationComp) modeling.None { return modeling.None{} },
			}
			c := def.Builder().WithSimulation(s).Build("GPU.SA.CU")
			require.Zero(t, calls)
			require.ErrorContains(t, s.Engine().Run(), "Initialize")
			path := filepath.Join(t.TempDir(), "snapshot.tar.gz")
			require.ErrorContains(t, s.SaveCheckpoint(path, "test"), "Initialize")
			require.NoFileExists(t, path)
			require.ErrorContains(t, s.Initialize(), "Top")
			require.Zero(t, calls)
			p := twowaybuffered.NewPort(2, 2)
			c.BindPort("Top", p)
			conn := direct.NewConnection("Link", s, timing.GHz)
			conn.BindPort(p)
			require.NoError(t, s.Initialize())
			require.True(t, c.State.SawConnection)
			require.Equal(t, 1, calls)
			require.Equal(t, "GPU.SA.CU.Top", p.Name())
			require.Same(t, c, p.Owner())
			require.Same(t, conn, p.Connection())
			require.NoError(t, s.Engine().Run())
			require.Error(t, s.Initialize())
			require.Panics(t, func() { def.Builder().WithSimulation(s).Build("Late") })
			require.Panics(t, func() { c.BindPort("Top", twowaybuffered.NewPort(1, 1)) })
			require.Panics(t, func() { direct.NewConnection("Late", s, timing.GHz) })
			require.Panics(t, func() { conn.BindPort(twowaybuffered.NewPort(1, 1)) })
			t.Log("Build left State untouched; initialization rejected missing Top, " +
				"then observed the completed link and froze topology")
		})
	}
}
