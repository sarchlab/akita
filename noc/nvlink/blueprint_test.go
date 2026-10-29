package nvlink_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/internal/networktest"
	"github.com/sarchlab/akita/v5/noc/nvlink"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

func TestBlueprintParityAndCheckpoint(t *testing.T) {
	networktest.CheckParity(t, []networktest.Case{{
		Name: "nvlink-root-device",
		Node: func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := nvlink.DefaultSpec()
			s.RootPorts = refs[:1]
			s.Devices = []nvlink.Device{{Switch: 0, Ports: refs[1:]}}
			s.NVLinks = []nvlink.Link{{A: 0, B: 1}}
			return nvlink.Node(s)
		}, Manual: func(s modeling.Simulation, p []messaging.Port) {
			c := nvlink.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(p[:1])
			b := c.ConnectDevice(root, p[1:])
			c.ConnectDevicesWithNVLink(0, b)
		}},
		{Name: "nvlink", Node: func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := nvlink.DefaultSpec()
			s.Devices = []nvlink.Device{{Switch: 0, Ports: refs[:1]}, {Switch: 0, Ports: refs[1:]}}
			s.NVLinks = []nvlink.Link{{A: 1, B: 2}}
			return nvlink.Node(s)
		}, Manual: func(s modeling.Simulation, p []messaging.Port) {
			c := nvlink.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(nil)
			a := c.ConnectDevice(root, p[:1])
			b := c.ConnectDevice(root, p[1:])
			c.ConnectDevicesWithNVLink(a, b)
		}}})
}

func TestInvalidBlueprintSettings(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("Network", nvlink.Node(nvlink.Spec{}))
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Network.Spec")
}

func TestBlueprintRejectsUnknownDeviceLink(t *testing.T) {
	spec := nvlink.DefaultSpec()
	spec.NVLinks = []nvlink.Link{{A: 0, B: 1}}
	var bp blueprint.Blueprint
	bp.Add("Network", nvlink.Node(spec))
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Network.Spec: NVLinks[0]")
}
