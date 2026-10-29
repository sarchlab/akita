package pcie_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/internal/networktest"
	"github.com/sarchlab/akita/v5/noc/pcie"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

func TestBlueprintParityAndCheckpoint(t *testing.T) {
	networktest.CheckParity(t, []networktest.Case{{
		Name: "pcie",
		Node: func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := pcie.DefaultSpec()
			s.RootPorts = refs[:1]
			s.Switches = []pcie.Switch{{Parent: -1}, {Parent: 0}}
			s.Devices = []pcie.Device{{Switch: 1, Ports: refs[1:]}}
			return pcie.Node(s)
		}, Manual: func(s modeling.Simulation, p []messaging.Port) {
			c := pcie.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(p[:1])
			child := c.AddSwitch(root)
			c.ConnectDevice(child, p[1:])
		}},
		{Name: "pcie-multiple-devices-and-transit", Node: func(refs []messaging.RemotePort) *blueprint.NetworkNode {
			s := pcie.DefaultSpec()
			s.Switches = []pcie.Switch{{Parent: -1}, {Parent: 0}, {Parent: 1}}
			s.Devices = []pcie.Device{{Switch: 2, Ports: refs[:1]}, {Switch: 2, Ports: refs[1:]}}
			return pcie.Node(s)
		}, Manual: func(s modeling.Simulation, p []messaging.Port) {
			c := pcie.MakeBuilder().WithSimulation(s).Build("Network")
			root := c.AddRootComplex(nil)
			transit := c.AddSwitch(root)
			child := c.AddSwitch(transit)
			c.ConnectDevice(child, p[:1])
			c.ConnectDevice(child, p[1:])
		}}})
}

func TestInvalidBlueprintSettings(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("Network", pcie.Node(pcie.Spec{}))
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Network.Spec")
}

func TestBlueprintRejectsUnknownDeviceSwitch(t *testing.T) {
	spec := pcie.DefaultSpec()
	spec.Switches = []pcie.Switch{{Parent: -1}}
	spec.Devices = []pcie.Device{{Switch: 1}}
	var bp blueprint.Blueprint
	bp.Add("Network", pcie.Node(spec))
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Network.Spec: Devices[0].Switch")
}
