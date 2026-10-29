package mesh_test

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/internal/networktest"
	"github.com/sarchlab/akita/v5/noc/mesh"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBlueprintParityAndCheckpoint(t *testing.T) {
	networktest.CheckParity(t, []networktest.Case{{
		Name: "mesh",
		Node: func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := mesh.DefaultSpec()
			s.SwitchLatency = 2
			s.Tiles = []mesh.Tile{{X: 0, Ports: refs[:1]}, {X: 2, Ports: refs[1:]}}
			return mesh.Node(s)
		}, Manual: func(s modeling.Simulation, p []messaging.Port) {
			c := mesh.MakeBuilder().WithSimulation(s).WithSwitchLatency(2).Build("Network")
			c.AddTile([3]int{}, p[:1])
			c.AddTile([3]int{2, 0, 0}, p[1:])
		}}})
}

type domainPorts struct{ Top messaging.Port }

func leaf() *blueprint.DomainNode {
	return blueprint.NewDomain[modeling.None, domainPorts](modeling.None{}, func(_ modeling.None,
		bp *blueprint.Blueprint) {
		bp.Add("Inner", networktest.Component(networktest.Spec{}))
		bp.Expose("Top", "Inner.Top")
	})
}

func TestMeshPreservesContendedDeviceBindingOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			run := func(declarative bool) ([]uint64, timing.VTimeInPicoSec) {
				s := sim.MakeBuilder().Build()
				defer s.Terminate()
				ports := make([]messaging.Port, 3)
				refs := []messaging.RemotePort{"A.Top", "B.Top"}
				if reverse {
					refs[0], refs[1] = refs[1], refs[0]
				}
				if declarative {
					for _, name := range []string{"A", "B", "C"} {
						s.Add(name, networktest.Component(networktest.Spec{}))
					}
					spec := mesh.DefaultSpec()
					spec.Tiles = []mesh.Tile{{Ports: refs}, {X: 1, Ports: []messaging.RemotePort{"C.Top"}}}
					s.Add("Network", mesh.Node(spec))
				} else {
					for i, name := range []string{"A", "B", "C"} {
						c := modeling.MakeBuilder[*networktest.Agent](networktest.Spec{}).WithSimulation(s).Build(name)
						ports[i] = twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(networktest.PortSpec()).Build()
						c.BindPort("Top", ports[i])
					}
					c := mesh.MakeBuilder().WithSimulation(s).Build("Network")
					pair := []messaging.Port{ports[0], ports[1]}
					if reverse {
						pair[0], pair[1] = pair[1], pair[0]
					}
					c.AddTile([3]int{}, pair)
					c.AddTile([3]int{1, 0, 0}, ports[2:])
				}
				require.NoError(t, s.Initialize())
				if declarative {
					for i, name := range []string{"A", "B", "C"} {
						ports[i] = s.Lookup(name).(*networktest.Agent).Ports.Top
					}
				}
				for i := range 2 {
					ports[i].Send(messaging.Msg{ID: uint64(i + 1), Src: ports[i].AsRemote(),
						Dst: ports[2].AsRemote(), TrafficBytes: 64})
				}
				require.NoError(t, s.Run())
				var ids []uint64
				for {
					m, ok := ports[2].RetrieveIncoming()
					if !ok {
						break
					}
					ids = append(ids, m.ID)
				}
				require.Len(t, ids, 2)
				return ids, s.Engine().CurrentTime()
			}
			want, when := run(false)
			got, at := run(true)
			require.Equal(t, want, got)
			require.Equal(t, when, at)
			if reverse {
				require.Equal(t, []uint64{2, 1}, got)
			} else {
				require.Equal(t, []uint64{1, 2}, got)
			}
			t.Logf("reverse=%v: contended arrivals %v, manual=blueprint time %d ps", reverse, got, at)
		})
	}
}

func TestNetworkMembersResolveExposedDomainAliases(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("Child", leaf())
	bp.Add("Peer", networktest.Component(networktest.Spec{}))
	spec := mesh.DefaultSpec()
	spec.Tiles = []mesh.Tile{
		{Ports: []messaging.RemotePort{bp.PortRef("Child.Top")}},
		{X: 1, Ports: []messaging.RemotePort{bp.PortRef("Peer.Top")}},
	}
	bp.Add("Network", mesh.Node(spec))
	flat, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	network := flat.Nodes()[2].(*blueprint.NetworkNode)
	require.Equal(t, []messaging.RemotePort{"Child.Inner.Top", "Peer.Top"}, network.PortReferences())
	resolved := network.Configuration().(mesh.Spec)
	require.Equal(t, messaging.RemotePort("Child.Inner.Top"), resolved.Tiles[0].Ports[0])
	require.Equal(t, messaging.RemotePort("Peer.Top"), resolved.Tiles[1].Ports[0])
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	for _, node := range bp.Nodes() {
		s.Add(node.Name(), node)
	}
	require.NoError(t, s.Initialize())
	ports := []messaging.Port{
		s.Lookup("Child").(domainPorts).Top,
		s.Lookup("Peer").(*networktest.Agent).Ports.Top,
	}
	networktest.Seed(s, ports)
	delivered := networktest.Finish(t, s, ports)
	require.Equal(t, ports[1].AsRemote(), delivered[0].Src)
	require.Equal(t, ports[0].AsRemote(), delivered[1].Src)
}

func TestInvalidBlueprintSettings(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("Network", mesh.Node(mesh.Spec{}))
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Network.Spec")
}
