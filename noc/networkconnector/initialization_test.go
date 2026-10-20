package networkconnector

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/stretchr/testify/require"
)

type countingRouter struct{ calls int }

func (r *countingRouter) EstablishRoute(nodes []Node) {
	r.calls++
	FloydWarshallRouter{}.EstablishRoute(nodes)
}

// Register topology resources after components to verify validation computes routes
// before component initialization in either registration order.
type lateResources struct {
	*sim.Simulation
	pending []naming.Named
}

func (s *lateResources) RegisterResource(r naming.Named) { s.pending = append(s.pending, r) }
func (s *lateResources) initialize() error {
	for _, r := range s.pending {
		s.Simulation.RegisterResource(r)
	}
	return s.Initialize()
}
func topologyParameters() (DeviceConfig, SwitchToSwitchLinkParameter) {
	end := LinkEndSwitchParameter{
		RouteBufferCapacity:   1,
		ForwardBufferCapacity: 1,
		SendBufferCapacity:    1,
		IncomingBufSize:       2,
		OutgoingBufSize:       2,
		NumInputChannel:       1,
		NumOutputChannel:      1,
		Latency:               2}
	return DeviceConfig{DeviceEndParam: LinkEndDeviceParameter{IncomingBufSize: 2,
			OutgoingBufSize:  2,
			NumInputChannel:  1,
			NumOutputChannel: 1},
			SwitchEndParam: end},
		SwitchToSwitchLinkParameter{LeftEndParam: end, RightEndParam: end}
}
func topologyPort(s modeling.Simulation, name string) messaging.Port {
	p := twowaybuffered.MakeBuilder().
		WithSimulation(s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 2, OutgoingCapacity: 2}).
		Build()
	p.BindOwner(routeTestOwner{}, name)
	s.RegisterPort(p)
	return p
}
func TestAutomaticInitializationFreezesRoutesAndAssembly(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint(late), func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			var service modeling.Simulation = s
			delayed := &lateResources{Simulation: s}
			if late {
				service = delayed
			}
			router := &countingRouter{}
			c := MakeBuilder().WithSimulation(service).WithRouter(router).Build("Network")
			a, b := c.AddSwitch(SwitchConfig{}), c.AddSwitch(SwitchConfig{})
			dp, sp := topologyParameters()
			src, dst := topologyPort(service, "Source.Port"), topologyPort(service, "Target.Port")
			c.ConnectDevice(a, []messaging.Port{src}, dp)
			c.ConnectDevice(b, []messaging.Port{dst}, dp)
			c.ConnectSwitches(a, b, sp)
			require.Nil(t, c.switches[a].Routes())
			require.Len(t, c.switches[a].sw.Ports.Links, 2)
			_ = (&networkCheckpoint{c: c}).record()
			require.Zero(t, router.calls)
			require.Len(t, c.switches[a].sw.Ports.Links, 2)
			for _, sw := range c.switches {
				for _, p := range sw.ports {
					require.NotEmpty(t, p.AsRemote())
					require.NotNil(t, p.Connection())
				}
			}
			if late {
				require.NoError(t, delayed.initialize())
			} else {
				require.NoError(t, s.Initialize())
			}
			require.Equal(t, 1, router.calls)
			output := c.switches[a].Routes().FindPort(dst.AsRemote())
			c.switches[a].table.DefineRoute(dst.AsRemote(), "NotABoundPort")
			require.Equal(t, output, c.switches[a].Routes().FindPort(dst.AsRemote()))
			require.Equal(t, 1, router.calls)
			require.Panics(t, func() { c.AddSwitch(SwitchConfig{}) })
			require.Panics(t, func() { c.ConnectSwitches(a, b, sp) })
			require.Panics(t, func() { c.ConnectDevice(a, nil, dp) })
			require.Len(t, c.switches, 2)
			t.Log("Connect calls bound all ports; Initialize computed routes once; " +
				"setup-table edits and late assembly cannot change runtime routing")
		})
	}
}

type alternateRouter struct{ alternate bool }

func (r alternateRouter) EstablishRoute(nodes []Node) {
	FloydWarshallRouter{}.EstablishRoute(nodes)
	if !r.alternate {
		return
	}
	for _, node := range nodes {
		if node.Name() != "Network.Switch[0]" {
			continue
		}
		for _, remote := range node.ListRemotes() {
			if remote.RemoteNode.Name() == "Network.Switch[3]" {
				node.Table().DefineRoute("Target.Port", remote.LocalPort.AsRemote())
			}
		}
	}
}
func checkpointNetwork(t *testing.T, router Router, reverse bool, latency int, reverseDevices bool) *sim.Simulation {
	t.Helper()
	s := sim.MakeBuilder().Build()
	c := MakeBuilder().WithSimulation(s).WithRouter(router).Build("Network")
	for range 4 {
		c.AddSwitch(SwitchConfig{})
	}
	dp, sp := topologyParameters()
	sp.LeftEndParam.Latency = latency
	source, extra, target := topologyPort(s, "Source.Port"), topologyPort(s, "Extra.Port"), topologyPort(s, "Target.Port")
	ports := []messaging.Port{source, extra}
	if reverseDevices {
		ports[0], ports[1] = ports[1], ports[0]
	}
	c.ConnectDevice(0, ports, dp)
	c.ConnectDevice(2, []messaging.Port{target}, dp)
	cycle := []int{0, 1, 2, 3}
	if reverse {
		cycle = []int{0, 3, 2, 1}
	}
	for i, a := range cycle {
		c.ConnectSwitches(a, cycle[(i+1)%4], sp)
	}
	require.NoError(t, s.Initialize())
	return s
}
func TestCheckpointRejectsChangedNetworkIdentity(t *testing.T) {
	baseline := checkpointNetwork(t, alternateRouter{}, false, 2, false)
	path := filepath.Join(t.TempDir(), "network.tar.gz")
	require.NoError(t, baseline.SaveCheckpoint(path))
	same := checkpointNetwork(t, alternateRouter{}, false, 2, false)
	require.NoError(t, same.LoadCheckpoint(path))
	renamedPolicy := checkpointNetwork(t, FloydWarshallRouter{}, false, 2, false)
	require.NoError(t, renamedPolicy.LoadCheckpoint(path))
	for _, tc := range []struct {
		name           string
		router         Router
		reverse        bool
		latency        int
		reverseDevices bool
	}{
		{"rewired-ports", alternateRouter{}, true, 2, false},
		{"computed-routes", alternateRouter{alternate: true}, false, 2, false},
		{"switch-port-latency", alternateRouter{}, false, 3, false},
		{"device-order", alternateRouter{}, false, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rebuilt := checkpointNetwork(t, tc.router, tc.reverse, tc.latency, tc.reverseDevices)
			err := rebuilt.LoadCheckpoint(path)
			require.ErrorContains(t, err, "spec hash mismatch")
			t.Logf("Checkpoint rejected %s: %v", tc.name, err)
		})
	}
}

type loopRouter struct{}

func (loopRouter) EstablishRoute(nodes []Node) {
	for _, n := range nodes {
		if n.Table() == nil {
			continue
		}
		for _, remote := range n.ListRemotes() {
			if remote.RemoteNode.Table() != nil {
				n.Table().DefineDefaultRoute(remote.LocalPort.AsRemote())
				break
			}
		}
	}
}
func TestInvalidTopologyFailsBeforeRun(t *testing.T) {
	for _, tc := range []struct {
		name    string
		router  Router
		connect bool
		want    string
	}{
		{"disconnected", FloydWarshallRouter{}, false, "disconnected"},
		{"loop", loopRouter{}, true, "routing loop"},
		{"invalid-next-hop", invalidHopRouter{}, true, "unreachable destination"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			c := MakeBuilder().WithSimulation(s).WithRouter(tc.router).Build("Network")
			a, b := c.AddSwitch(SwitchConfig{}), c.AddSwitch(SwitchConfig{})
			dp, sp := topologyParameters()
			c.ConnectDevice(a, []messaging.Port{topologyPort(s, "Source.Port")}, dp)
			c.ConnectDevice(b, []messaging.Port{topologyPort(s, "Target.Port")}, dp)
			if tc.connect {
				c.ConnectSwitches(a, b, sp)
			}
			require.ErrorContains(t, s.Initialize(), tc.want)
			require.Error(t, s.Run())
			require.Error(t, s.Initialize())
		})
	}
}

type invalidHopRouter struct{}

func (invalidHopRouter) EstablishRoute(nodes []Node) {
	for _, n := range nodes {
		if n.Table() != nil {
			n.Table().DefineDefaultRoute("NotALocalPort")
		}
	}
}

func TestValidationCanBeRetriedAfterCompletingWiring(t *testing.T) {
	s := sim.MakeBuilder().Build()
	c := MakeBuilder().WithSimulation(s).Build("Network")
	a, b := c.AddSwitch(SwitchConfig{}), c.AddSwitch(SwitchConfig{})
	dp, sp := topologyParameters()
	c.ConnectDevice(a, []messaging.Port{topologyPort(s, "Source.Port")}, dp)
	c.ConnectDevice(b, []messaging.Port{topologyPort(s, "Target.Port")}, dp)
	require.ErrorContains(t, s.Initialize(), "disconnected")
	require.Nil(t, c.routes)
	c.ConnectSwitches(a, b, sp)
	require.NoError(t, s.Initialize())
	require.NotEmpty(t, c.switches[a].Routes().FindPort("Target.Port"))
	t.Log("Disconnected wiring returns an error; completing wiring and retrying Initialize computes valid routes")
}

func TestSwitchUsesIndependentBufferSizes(t *testing.T) {
	s := sim.MakeBuilder().Build()
	c := MakeBuilder().WithSimulation(s).Build("Network")
	a, b := c.AddSwitch(SwitchConfig{}), c.AddSwitch(SwitchConfig{})
	_, sp := topologyParameters()
	sp.LeftEndParam.IncomingBufSize = 3
	sp.LeftEndParam.OutgoingBufSize = 1
	c.ConnectSwitches(a, b, sp)
	require.NoError(t, s.Initialize())
	p := c.switches[a].ports[0]
	for range 3 {
		require.True(t, p.CanDeliver())
		p.Deliver(messaging.Msg{ID: s.NewID()})
	}
	require.False(t, p.CanDeliver())
	require.True(t, p.CanSend())
	p.Send(messaging.Msg{ID: s.NewID(), Src: p.AsRemote(), Dst: c.switches[b].ports[0].AsRemote()})
	require.False(t, p.CanSend())
	t.Log("Asymmetric switch port holds three incoming messages and one outgoing message")
}

func TestRepeatedEndpointRequiresMatchingConfiguration(t *testing.T) {
	s := sim.MakeBuilder().Build()
	c := MakeBuilder().WithSimulation(s).Build("Network")
	a, b := c.AddSwitch(SwitchConfig{}), c.AddSwitch(SwitchConfig{})
	dp, _ := topologyParameters()
	dp.Name = "EP"
	c.ConnectDevice(a, nil, dp)
	require.Panics(t, func() { c.ConnectDevice(b, nil, dp) })
	dp.SwitchEndParam.Latency++
	require.Panics(t, func() { c.ConnectDevice(a, nil, dp) })
	require.Len(t, c.devices, 1)
}
