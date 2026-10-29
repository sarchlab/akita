package blueprint_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/noc/mesh"
	"github.com/sarchlab/akita/v5/noc/nvlink"
	"github.com/sarchlab/akita/v5/noc/pcie"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type networkCase struct {
	name   string
	node   func([]messaging.RemotePort) *blueprint.NetworkNode
	manual func(modeling.Simulation, []messaging.Port)
}

func networkCases() []networkCase {
	return []networkCase{
		{"mesh", func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := mesh.DefaultSpec()
			s.SwitchLatency = 2
			s.Tiles = []mesh.Tile{{X: 0, Ports: refs[:1]}, {X: 2, Ports: refs[1:]}}
			return mesh.Node(s)
		}, func(s modeling.Simulation, p []messaging.Port) {
			c := mesh.MakeBuilder().WithSimulation(s).WithSwitchLatency(2).Build("Network")
			c.AddTile([3]int{}, p[:1])
			c.AddTile([3]int{2, 0, 0}, p[1:])
		}},
		{"pcie", func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := pcie.DefaultSpec()
			s.RootPorts = refs[:1]
			s.Switches = []pcie.Switch{{Parent: -1}, {Parent: 0}}
			s.Devices = []pcie.Device{{Switch: 1, Ports: refs[1:]}}
			return pcie.Node(s)
		}, func(s modeling.Simulation, p []messaging.Port) {
			c := pcie.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(p[:1])
			child := c.AddSwitch(root)
			c.ConnectDevice(child, p[1:])
		}},
		{"pcie-multiple-devices-and-transit", func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := pcie.DefaultSpec()
			s.Switches = []pcie.Switch{{Parent: -1}, {Parent: 0}, {Parent: 1}}
			s.Devices = []pcie.Device{{Switch: 2, Ports: refs[:1]}, {Switch: 2, Ports: refs[1:]}}
			return pcie.Node(s)
		}, func(s modeling.Simulation, p []messaging.Port) {
			c := pcie.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(nil)
			transit := c.AddSwitch(root)
			child := c.AddSwitch(transit)
			c.ConnectDevice(child, p[:1])
			c.ConnectDevice(child, p[1:])
		}},
		{"nvlink-root-device", func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := nvlink.DefaultSpec()
			s.RootPorts = refs[:1]
			s.Devices = []nvlink.Device{{Switch: 0, Ports: refs[1:]}}
			s.NVLinks = []nvlink.Link{{A: 0, B: 1}}
			return nvlink.Node(s)
		}, func(s modeling.Simulation, p []messaging.Port) {
			c := nvlink.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(p[:1])
			b := c.ConnectDevice(root, p[1:])
			c.ConnectDevicesWithNVLink(0, b)
		}},
		{"nvlink", func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := nvlink.DefaultSpec()
			s.Devices = []nvlink.Device{{Switch: 0, Ports: refs[:1]}, {Switch: 0, Ports: refs[1:]}}
			s.NVLinks = []nvlink.Link{{A: 1, B: 2}}
			return nvlink.Node(s)
		}, func(s modeling.Simulation, p []messaging.Port) {
			c := nvlink.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(nil)
			a := c.ConnectDevice(root, p[:1])
			b := c.ConnectDevice(root, p[1:])
			c.ConnectDevicesWithNVLink(a, b)
		}},
	}
}
func buildNetwork(t *testing.T, tc networkCase, declarative bool) (*sim.Simulation, []messaging.Port) {
	t.Helper()
	s := sim.MakeBuilder().Build()
	ports := make([]messaging.Port, 2)
	if declarative {
		s.Add("", blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
			bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}))
			bp.Add("B", comp(testSpec{}))
			bp.Add("Network", tc.node([]messaging.RemotePort{bp.PortRef("A.Top"), bp.PortRef("B.Top")}))
		}))
	} else {
		for i, name := range []string{"A", "B"} {
			c := modeling.MakeBuilder[*testComp](testSpec{}).WithSimulation(s).Build(name)
			p := twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(portSpec()).Build()
			c.BindPort("Top", p)
			ports[i] = p
		}
		tc.manual(s, ports)
	}
	require.NoError(t, s.Initialize())
	if declarative {
		ports[0] = blueprint.Lookup[*testComp](s, "A").Ports.Top
		ports[1] = blueprint.Lookup[*testComp](s, "B").Ports.Top
	}
	return s, ports
}
func seedNetwork(s *sim.Simulation, p []messaging.Port) {
	for i := range 2 {
		p[i].Send(messaging.Msg{ID: s.NewID(), Src: p[i].AsRemote(), Dst: p[1-i].AsRemote(), TrafficBytes: 64})
	}
}
func finishNetwork(t *testing.T, s *sim.Simulation, p []messaging.Port) []messaging.Msg {
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
func TestNetworkParityAndCheckpoint(t *testing.T) {
	for _, tc := range networkCases() {
		t.Run(tc.name, func(t *testing.T) {
			manual, mp := buildNetwork(t, tc, false)
			defer manual.Terminate()
			seedNetwork(manual, mp)
			expected := finishNetwork(t, manual, mp)
			declared, dp := buildNetwork(t, tc, true)
			defer declared.Terminate()
			seedNetwork(declared, dp)
			require.NoError(t, declared.Engine().(*timing.SerialEngine).RunUntil(1000))
			path := filepath.Join(t.TempDir(), "network.checkpoint")
			require.NoError(t, declared.SaveCheckpoint(path))
			actual := finishNetwork(t, declared, dp)
			require.Equal(t, expected, actual)
			require.Equal(t, manual.Engine().CurrentTime(), declared.Engine().CurrentTime())
			restored, rp := buildNetwork(t, tc, true)
			defer restored.Terminate()
			require.NoError(t, restored.LoadCheckpoint(path))
			require.Equal(t, actual, finishNetwork(t, restored, rp))
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
				"identical internal component names", tc.name, declared.Engine().CurrentTime())
		})
	}
}
func TestCustomConnectorIsDeferred(t *testing.T) {
	called := false
	var bp blueprint.Blueprint
	bp.Add("A", comp(testSpec{}))
	bp.Add("B", comp(testSpec{}))
	n := blueprint.NewNetwork(struct{ Freq timing.Freq }{timing.GHz}, []messaging.RemotePort{bp.PortRef("A.Top"),
		bp.PortRef("B.Top")}, func(settings struct{ Freq timing.Freq }, s modeling.Simulation, name string,
		ports []messaging.Port) {
		called = true
		require.Equal(t, "A.Top", ports[0].Name())
		require.Equal(t, "B.Top", ports[1].Name())
		c := direct.NewConnection(name, s, settings.Freq)
		for _, p := range ports {
			c.BindPort(p)
		}
	})
	bp.Add("Custom", n)
	flat, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	require.False(t, called)
	require.IsType(t, &blueprint.NetworkNode{}, flat.Nodes()[2].Node)
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	for _, n := range bp.Nodes() {
		s.Add(n.Name, n.Node)
	}
	require.NoError(t, s.Initialize())
	require.True(t, called)
}
func TestInvalidNetworkFailsBeforeInstantiation(t *testing.T) {
	for _, tc := range []struct {
		name string
		node *blueprint.NetworkNode
	}{
		{"Mesh", mesh.Node(mesh.Spec{})}, {"PCIe", pcie.Node(pcie.DefaultSpec())},
		{"PCIeDevice", pcie.Node(pcie.Spec{Freq: timing.GHz, FlitSize: 34, InputWidth: 1, OutputWidth: 1,
			Switches: []pcie.Switch{{Parent: -1}}, Devices: []pcie.Device{{Switch: 1}}})},
		{"NVLinkDevice", nvlink.Node(nvlink.Spec{Freq: timing.GHz, FlitSize: 34, InputWidth: 1, OutputWidth: 1,
			NVLinks: []nvlink.Link{{A: 0, B: 1}}})}, {"NVLink",
			nvlink.Node(nvlink.Spec{Freq: timing.GHz, FlitSize: 16, InputWidth: 1, OutputWidth: 1,
				Devices: []nvlink.Device{{Switch: 9}}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bp blueprint.Blueprint
			bp.Add(tc.name, tc.node)
			_, err := blueprint.Expand(&bp)
			require.ErrorContains(t, err, tc.name+".Spec")
		})
	}
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
						s.Add(name, comp(testSpec{}))
					}
					spec := mesh.DefaultSpec()
					spec.Tiles = []mesh.Tile{{Ports: refs}, {X: 1, Ports: []messaging.RemotePort{"C.Top"}}}
					s.Add("Network", mesh.Node(spec))
				} else {
					for i, name := range []string{"A", "B", "C"} {
						c := modeling.MakeBuilder[*testComp](testSpec{}).WithSimulation(s).Build(name)
						ports[i] = twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(portSpec()).Build()
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
						ports[i] = blueprint.Lookup[*testComp](s, name).Ports.Top
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
	bp.Add("Child", leaf(domainSpec{}))
	bp.Add("Peer", comp(testSpec{}))
	spec := mesh.DefaultSpec()
	spec.Tiles = []mesh.Tile{
		{Ports: []messaging.RemotePort{bp.PortRef("Child.Top")}},
		{X: 1, Ports: []messaging.RemotePort{bp.PortRef("Peer.Top")}},
	}
	bp.Add("Network", mesh.Node(spec))
	flat, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	network := flat.Nodes()[2].Node.(*blueprint.NetworkNode)
	require.Equal(t, []messaging.RemotePort{"Child.Inner.Top", "Peer.Top"}, network.PortReferences())
	resolved := network.Configuration().(mesh.Spec)
	require.Equal(t, messaging.RemotePort("Child.Inner.Top"), resolved.Tiles[0].Ports[0])
	require.Equal(t, messaging.RemotePort("Peer.Top"), resolved.Tiles[1].Ports[0])
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	for _, node := range bp.Nodes() {
		s.Add(node.Name, node.Node)
	}
	require.NoError(t, s.Initialize())
	ports := []messaging.Port{
		blueprint.Lookup[domainPorts](s, "Child").Top,
		blueprint.Lookup[*testComp](s, "Peer").Ports.Top,
	}
	seedNetwork(s, ports)
	delivered := finishNetwork(t, s, ports)
	require.Equal(t, ports[1].AsRemote(), delivered[0].Src)
	require.Equal(t, ports[0].AsRemote(), delivered[1].Src)
}
