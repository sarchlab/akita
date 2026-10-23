package networkconnector_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"

	"github.com/sarchlab/akita/v5/noc/internal/endpoint"
	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/noc/mesh"
	"github.com/sarchlab/akita/v5/noc/nvlink"
	"github.com/sarchlab/akita/v5/noc/pcie"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// Exercise topology assembly and delivery with unequal input/output widths.
// Network-owned queues follow those widths; device-owned queues stay unchanged.
func TestTopologyDirectConfigurationAndDelivery(t *testing.T) {
	for _, topology := range []string{"pcie", "nvlink", "mesh"} {
		t.Run(topology, func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			t.Cleanup(func() { require.NoError(t, s.Terminate()) })
			ports := make([]messaging.Port, 2)
			for i, name := range []string{"Source", "Destination"} {
				p := twowaybuffered.MakeBuilder().WithSimulation(s).
					WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).Build()
				p.BindOwner(payloadOwner{name}, name+".Port")
				s.RegisterPort(p)
				ports[i] = p
			}
			switch topology {
			case "pcie":
				c := pcie.MakeBuilder().WithSimulation(s).WithFreq(2 * timing.GHz).WithFlitSize(32).
					WithInputWidth(3).WithOutputWidth(2).
					Build("Network")
				a := c.AddRootComplex(ports[:1])
				c.ConnectDevice(c.AddSwitch(a), ports[1:])
			case "nvlink":
				c := nvlink.MakeBuilder().WithSimulation(s).WithFreq(2 * timing.GHz).WithFlitSize(32).
					WithInputWidth(3).WithOutputWidth(2).
					Build("Network")
				a, middle, b := c.AddSwitch(), c.AddSwitch(), c.AddSwitch()
				c.ConnectSwitchesWithPCIeLink(a, middle)
				c.ConnectSwitchesWithPCIeLink(middle, b)
				d0, d1 := c.ConnectDevice(a, ports[:1]), c.ConnectDevice(b, ports[1:])
				c.ConnectDevicesWithNVLink(d0, d1)
			case "mesh":
				c := mesh.MakeBuilder().WithSimulation(s).WithFreq(2 * timing.GHz).WithFlitSize(32).
					WithInputWidth(3).WithOutputWidth(2).
					Build("Network")
				c.AddTile([3]int{0, 0, 0}, ports[:1])
				c.AddTile([3]int{2, 0, 0}, ports[1:])
			}
			require.NoError(t, s.Initialize())
			for _, comp := range s.Components() {
				switch c := comp.(type) {
				case *switches.Comp:
					for i, pc := range c.State.PortComplexes {
						require.Equal(t, 3, pc.NumInputChannel)
						require.Equal(t, 2, pc.NumOutputChannel)
						require.Equal(t, 3, pc.RouteBuffer.Capacity())
						require.Equal(t, 3, pc.ForwardBuffer.Capacity())
						require.Equal(t, 2, pc.SendOutBuffer.Capacity())
						require.Equal(t, 3, c.Ports.Links[i].IncomingBuf().Capacity())
						require.Equal(t, 2, c.Ports.Links[i].OutgoingBuf().Capacity())
					}
					if topology == "nvlink" && c.Name() == "Network.DeviceSwitch[0]" {
						// The NVLink path is three switch-to-switch hops; PCIe is
						// four. Its first port leads to the attached NVLink switch.
						require.Equal(t, c.Ports.Links[0].AsRemote(),
							c.Resources.Wiring.Routes().FindPort(ports[1].AsRemote()))
					}
				case *endpoint.Comp:
					require.Equal(t, 32, c.Spec.FlitByteSize)
					require.Equal(t, 2*timing.GHz, c.Spec.Freq)
					require.Equal(t, 3, c.Spec.NumInputChannels)
					require.Equal(t, 2, c.Spec.NumOutputChannels)
					require.Equal(t, 3, c.Ports.NetworkPort.IncomingBuf().Capacity())
					require.Equal(t, 2, c.Ports.NetworkPort.OutgoingBuf().Capacity())
				}
			}
			for i := range 2 {
				require.Equal(t, 4, ports[i].IncomingBuf().Capacity())
				require.Equal(t, 4, ports[i].OutgoingBuf().Capacity())
				ports[i].Send(messaging.Msg{ID: s.NewID(), Src: ports[i].AsRemote(),
					Dst: ports[1-i].AsRemote(), TrafficBytes: 128, Payload: memprotocol.ReadReq{Address: 256, AccessByteSize: 128}})
			}
			require.NoError(t, s.Run())
			for _, p := range ports {
				msg, ok := p.RetrieveIncoming()
				require.True(t, ok)
				require.Equal(t, memprotocol.ReadReq{Address: 256, AccessByteSize: 128}, msg.Payload)
				_, extra := p.RetrieveIncoming()
				require.False(t, extra)
			}
		})
	}
}
