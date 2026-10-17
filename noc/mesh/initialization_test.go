package mesh

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type observedSimulation struct {
	*sim.Simulation
	switches map[string]*switches.Comp
}

func (s *observedSimulation) RegisterComponent(c naming.Named) {
	s.Simulation.RegisterComponent(c)
	if sw, ok := c.(*switches.Comp); ok {
		s.switches[c.Name()] = sw
	}
}

type meshOwner struct{ name string }

func (o meshOwner) Name() string                { return o.name }
func (meshOwner) NotifyRecv(messaging.Port)     {}
func (meshOwner) NotifyPortFree(messaging.Port) {}
func makeMesh(t *testing.T, farFirst bool) (*observedSimulation, *Connector, []messaging.Port) {
	t.Helper()
	s := &observedSimulation{Simulation: sim.MakeBuilder().Build(), switches: make(map[string]*switches.Comp)}
	c := MakeBuilder().WithSimulation(s).WithSwitchLatency(2).Build("Mesh")
	ports := make([]messaging.Port, 0, 3)
	for _, name := range []string{"Origin", "Extra", "Far"} {
		p := twowaybuffered.NewPort(8, 8)
		p.BindOwner(meshOwner{name}, name+".Port")
		s.RegisterPort(p)
		ports = append(ports, p)
	}
	origin := func() { c.AddTile([3]int{0, 0, 0}, ports[:1]) }
	far := func() { c.AddTile([3]int{2, 2, 2}, ports[2:]) }
	if farFirst {
		far()
		origin()
	} else {
		origin()
		far()
	}
	c.AddTile([3]int{0, 0, 0}, ports[1:2])
	require.Len(t, c.tiles, 27)
	require.Len(t, c.tiles[[3]int{}].ports, 2)
	require.NoError(t, s.Initialize())
	return s, c, ports
}
func sendMeshTraffic(s *observedSimulation, ports []messaging.Port) {
	for i := range 2 {
		ports[i].Send(messaging.Msg{ID: s.NewID(), Src: ports[i].AsRemote(), Dst: ports[2].AsRemote(), TrafficBytes: 64})
		ports[2].Send(messaging.Msg{ID: s.NewID(), Src: ports[2].AsRemote(), Dst: ports[i].AsRemote(), TrafficBytes: 64})
	}
}
func finishMesh(t *testing.T, s *observedSimulation, ports []messaging.Port) [][]messaging.Msg {
	t.Helper()
	require.NoError(t, s.Run())
	messages := make([][]messaging.Msg, len(ports))
	for i, p := range ports {
		for {
			msg, ok := p.RetrieveIncoming()
			if !ok {
				break
			}
			messages[i] = append(messages[i], msg)
		}
	}
	require.Len(t, messages[0], 1)
	require.Len(t, messages[1], 1)
	require.Len(t, messages[2], 2)
	return messages
}
func TestIncrementalMeshOrderDeliveryAndCheckpoint(t *testing.T) {
	for _, farFirst := range []bool{false, true} {
		s, c, ports := makeMesh(t, farFirst)
		sw := s.switches["Mesh.SW[1][1][1]"]
		require.Equal(t, sw.Ports.Links, sw.Resources.Wiring.Ports())
		sendMeshTraffic(s, ports)
		require.NoError(t, s.Engine().(*timing.SerialEngine).RunUntil(6000))
		path := filepath.Join(t.TempDir(), "mesh.tar.gz")
		require.NoError(t, s.SaveCheckpoint(path))
		uninterrupted := finishMesh(t, s, ports)
		restored, _, restoredPorts := makeMesh(t, farFirst)
		require.NoError(t, restored.LoadCheckpoint(path))
		require.Equal(t, uninterrupted, finishMesh(t, restored, restoredPorts))
		require.Equal(t, s.Engine().CurrentTime(), restored.Engine().CurrentTime())
		require.Panics(t, func() { c.AddTile([3]int{3, 3, 3}, nil) })
		require.Len(t, c.tiles, 27)
		t.Logf("far-first=%v: 27 tiles, repeated-tile merge, all four messages delivered; "+
			"checkpoint resumes at identical %d ps", farFirst, s.Engine().CurrentTime())
	}
}
