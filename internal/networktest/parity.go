// Package networktest provides connector-independent parity and checkpoint checks.
package networktest

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type Spec struct{}

type Ports struct{ Top messaging.Port }

type Agent = wakeup.Component[Spec, modeling.None, modeling.None, Ports, modeling.None]

func PortSpec() twowaybuffered.Spec {
	return twowaybuffered.Spec{IncomingCapacity: 3, OutgoingCapacity: 7}
}

func Component(spec Spec) *blueprint.ComponentNode {
	return blueprint.Comp[*Agent](spec).Port("Top", PortSpec())
}

type Case struct {
	Name   string
	Node   func([]messaging.RemotePort) *blueprint.NetworkNode
	Manual func(modeling.Simulation, []messaging.Port)
}

func Build(t *testing.T, tc Case, declarative bool) (*sim.Simulation, []messaging.Port) {
	t.Helper()
	s := sim.MakeBuilder().Build()
	ports := make([]messaging.Port, 2)
	if declarative {
		s.Add("", blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
			bp *blueprint.Blueprint) {
			bp.Add("A", Component(Spec{}))
			bp.Add("B", Component(Spec{}))
			bp.Add("Network", tc.Node([]messaging.RemotePort{bp.PortRef("A.Top"), bp.PortRef("B.Top")}))
		}))
	} else {
		for i, name := range []string{"A", "B"} {
			c := modeling.MakeBuilder[*Agent](Spec{}).WithSimulation(s).Build(name)
			p := twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(PortSpec()).Build()
			c.BindPort("Top", p)
			ports[i] = p
		}
		tc.Manual(s, ports)
	}
	require.NoError(t, s.Initialize())
	if declarative {
		ports[0] = s.Lookup("A").(*Agent).Ports.Top
		ports[1] = s.Lookup("B").(*Agent).Ports.Top
	}
	return s, ports
}

func Seed(s *sim.Simulation, p []messaging.Port) {
	for i := range 2 {
		p[i].Send(messaging.Msg{ID: s.NewID(), Src: p[i].AsRemote(), Dst: p[1-i].AsRemote(), TrafficBytes: 64})
	}
}

func Finish(t *testing.T, s *sim.Simulation, p []messaging.Port) []messaging.Msg {
	t.Helper()
	require.NoError(t, s.Run())
	out := make([]messaging.Msg, 2)
	for i := range 2 {
		m, ok := p[i].RetrieveIncoming()
		require.True(t, ok)
		out[i] = m
	}
	return out
}

func CheckParity(t *testing.T, cases []Case) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			manual, mp := Build(t, tc, false)
			defer manual.Terminate()
			Seed(manual, mp)
			expected := Finish(t, manual, mp)
			declared, dp := Build(t, tc, true)
			defer declared.Terminate()
			Seed(declared, dp)
			require.NoError(t, declared.Engine().(*timing.SerialEngine).RunUntil(1000))
			path := filepath.Join(t.TempDir(), "network.checkpoint")
			require.NoError(t, declared.SaveCheckpoint(path))
			actual := Finish(t, declared, dp)
			require.Equal(t, expected, actual)
			require.Equal(t, manual.Engine().CurrentTime(), declared.Engine().CurrentTime())
			restored, rp := Build(t, tc, true)
			defer restored.Terminate()
			require.NoError(t, restored.LoadCheckpoint(path))
			require.Equal(t, actual, Finish(t, restored, rp))
			require.Equal(t, declared.Engine().CurrentTime(), restored.Engine().CurrentTime())
			mn := make([]string, 0, len(manual.Components()))
			dn := make([]string, 0, len(declared.Components()))
			for _, c := range manual.Components() {
				mn = append(mn, c.Name())
			}
			for _, c := range declared.Components() {
				dn = append(dn, c.Name())
			}
			require.Equal(t, mn, dn)
			t.Logf("%s: two 64-byte messages delivered; manual=blueprint=restored time %d ps; "+
				"identical internal component names", tc.Name, declared.Engine().CurrentTime())
		})
	}
}
