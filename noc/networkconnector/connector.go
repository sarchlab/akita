package networkconnector

import (
	"fmt"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/noc/internal/endpoint"
	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// LinkEndSwitchParameter defines the parameters of the end of a link that is
// connected to a switch. Widths are positive flits per cycle; latency is
// nonnegative switch-processing cycles. Internal input/output queues are sized
// to their respective widths.
type LinkEndSwitchParameter struct {
	InputWidth  int
	OutputWidth int
	Latency     int
}

// LinkEndDeviceParameter defines the parameter that associated with an end of a
// link that is connected to a device. Widths are positive flits per cycle and
// size the endpoint network port buffers; caller-owned device ports are unchanged.
type LinkEndDeviceParameter struct {
	InputWidth  int
	OutputWidth int
}

// DeviceConfig contains the parameters that define a link
// between a device and a switch.
type DeviceConfig struct {
	Name           string // Empty selects an automatically generated endpoint name.
	DeviceEndParam LinkEndDeviceParameter
	SwitchEndParam LinkEndSwitchParameter
}

// SwitchToSwitchLinkParameter contains the parameters that define a link
// between two switches.
type SwitchToSwitchLinkParameter struct {
	LeftEndParam  LinkEndSwitchParameter
	RightEndParam LinkEndSwitchParameter
}

// Connector builds networks with twowaybuffered.Port and direct.Connection.
// Device ports must also be *twowaybuffered.Port; wire ports are not supported.
type Connector struct {
	name        string
	simulation  modeling.Simulation
	defaultFreq timing.Freq
	flitSize    int
	router      Router

	switches        []*switchNode
	devices         []*deviceNode
	connectionCount int
	routes          map[string]frozenRoutes
}

// Builder configures independent network assembly instances.
type Builder struct {
	simulation modeling.Simulation
	freq       timing.Freq
	flitSize   int
	router     Router
}

func MakeBuilder() Builder {
	return Builder{freq: timing.GHz, flitSize: 64, router: FloydWarshallRouter{}}
}
func (b Builder) WithSimulation(s modeling.Simulation) Builder { b.simulation = s; return b }
func (b Builder) WithFreq(f timing.Freq) Builder               { b.freq = f; return b }
func (b Builder) WithFlitSize(size int) Builder                { b.flitSize = size; return b }
func (b Builder) WithRouter(router Router) Builder             { b.router = router; return b }

// Build begins an independent network with the supplied name.
func (b Builder) Build(name string) *Connector {
	naming.MustBeValid(name)
	if b.simulation == nil {
		panic("networkconnector: WithSimulation is required")
	}
	assembly.RequireSetup(b.simulation)
	if b.freq <= 0 || b.flitSize <= 0 {
		panic("networkconnector: frequency and flit size must be positive")
	}
	c := &Connector{
		name: name, simulation: b.simulation,
		defaultFreq: b.freq, flitSize: b.flitSize, router: b.router,
	}
	b.simulation.RegisterResource(&networkSetup{
		Member: assembly.NewMember(b.simulation, c.initializeRoutes, nil), c: c,
	})
	return c
}

// SwitchConfig configures a switch's setup name and optional routing table.
// An empty name is generated; a nil table creates a fresh table. Tables are
// populated by the network router unless WithRouter(nil) is used.
type SwitchConfig struct {
	Name         string
	RoutingTable *routing.Table
}

// AddSwitch creates a switch during network setup.
func (c *Connector) AddSwitch(config SwitchConfig) int {
	assembly.RequireSetup(c.simulation)
	id := len(c.switches)
	name := config.Name
	if name == "" {
		name = fmt.Sprintf("Switch[%d]", id)
	}
	table := config.RoutingTable
	if table == nil {
		table = routing.NewTable()
	}
	spec := switches.DefaultSpec()
	spec.Freq = c.defaultFreq
	node := &switchNode{name: fmt.Sprintf("%s.%s", c.name, name), table: table, network: c}
	node.sw = modeling.MakeBuilder[*switches.Comp](spec).WithSimulation(c.simulation).
		WithResources(switches.Resources{Wiring: node}).Build(node.name)
	c.switches = append(c.switches, node)
	return id
}

// ConnectDevice attaches device ports through an endpoint. Reusing an explicit
// name with the same switch and parameters appends ports in binding order.
func (c *Connector) ConnectDevice(switchID int, ports []messaging.Port, param DeviceConfig) {
	assembly.RequireSetup(c.simulation)
	epName := param.Name
	if epName == "" {
		epName = fmt.Sprintf("EndPoint[%d]", len(c.devices))
	}
	swNode := c.switches[switchID]
	epFullName := fmt.Sprintf("%s.%s", c.name, epName)

	for _, device := range c.devices {
		if device.endPoint.Name() != epFullName {
			continue
		}
		if device.sw != swNode || device.parameter != param {
			panic("networkconnector: repeated endpoint name needs the same switch and parameters")
		}
		for _, port := range ports {
			endpoint.DeviceConnection(device.endPoint).BindPort(port)
		}
		device.ports = append(device.ports, ports...)
		return
	}
	epPort := twowaybuffered.MakeBuilder().
		WithSimulation(c.simulation).
		WithSpec(twowaybuffered.Spec{
			IncomingCapacity: param.DeviceEndParam.InputWidth,
			OutgoingCapacity: param.DeviceEndParam.OutputWidth,
		}).
		Build()
	swPort := swNode.addPort(param.SwitchEndParam)

	epNode := c.createEndPoint(epFullName, ports, param, swNode, epPort, swPort)
	conn := c.connectPorts(epPort, swPort)
	c.createRemoteInfoFoEP(epNode, swNode, epPort, swPort, conn)
}

// createEndPoint builds the endpoint that carries the device ports' messages
// over the link from epPort to swPort.
func (c *Connector) createEndPoint(
	name string,
	ports []messaging.Port,
	param DeviceConfig,
	swNode *switchNode,
	epPort, swPort messaging.Port,
) *deviceNode {
	epSpec := endpoint.DefaultSpec()
	epSpec.Freq = c.defaultFreq
	epSpec.FlitByteSize = c.flitSize
	epSpec.NumInputChannels = param.DeviceEndParam.InputWidth
	epSpec.NumOutputChannels = param.DeviceEndParam.OutputWidth
	endPoint := endpoint.MakeBuilder().
		WithSimulation(c.simulation).
		WithSpec(epSpec).
		WithPeer(swPort).
		Build(name)

	endPoint.BindPort("NetworkPort", epPort)
	for _, port := range ports {
		endpoint.DeviceConnection(endPoint).BindPort(port)
	}

	epNode := &deviceNode{
		ports:     append([]messaging.Port(nil), ports...),
		parameter: param,
		endPoint:  endPoint,
		sw:        swNode,
	}
	c.devices = append(c.devices, epNode)

	return epNode
}

func (c *Connector) createRemoteInfoFoEP(
	epNode *deviceNode, swNode *switchNode,
	epPort, swPort messaging.Port,
	conn *direct.Connection,
) {
	epNode.remote = Remote{
		LocalNode:  epNode,
		LocalPort:  epPort,
		RemoteNode: swNode,
		RemotePort: swPort,
		Link:       conn,
	}
	swNode.remotes = append(swNode.remotes, Remote{
		LocalNode:  swNode,
		LocalPort:  swPort,
		RemoteNode: epNode,
		RemotePort: epPort,
		Link:       conn,
	})
}

func (c *Connector) connectPorts(
	left, right messaging.Port,
) (conn *direct.Connection) {
	connName := fmt.Sprintf("%s.Conn[%d]", c.name, c.connectionCount)
	c.connectionCount++

	conn = direct.NewConnection(connName, c.simulation, c.defaultFreq)

	conn.BindPort(left)
	conn.BindPort(right)

	return conn
}

// ConnectSwitches create a connection between two switches. The connection
// created is bi-directional.
func (c *Connector) ConnectSwitches(
	leftSwitchID, rightSwitchID int,
	param SwitchToSwitchLinkParameter,
) {
	assembly.RequireSetup(c.simulation)
	leftNode := c.switches[leftSwitchID]
	rightNode := c.switches[rightSwitchID]

	leftPort := leftNode.addPort(param.LeftEndParam)
	rightPort := rightNode.addPort(param.RightEndParam)

	conn := c.connectPorts(leftPort, rightPort)

	c.createRemoteInfo(leftNode, rightNode, leftPort, rightPort, conn)
}

func (c *Connector) createRemoteInfo(
	leftNode, rightNode *switchNode,
	leftPort, rightPort messaging.Port,
	conn *direct.Connection,
) {
	leftNode.remotes = append(leftNode.remotes, Remote{
		LocalNode:  leftNode,
		LocalPort:  leftPort,
		RemoteNode: rightNode,
		RemotePort: rightPort,
		Link:       conn,
	})

	rightNode.remotes = append(rightNode.remotes, Remote{
		LocalNode:  rightNode,
		LocalPort:  rightPort,
		RemoteNode: leftNode,
		RemotePort: leftPort,
		Link:       conn,
	})
}

func (c *Connector) createRoutingNodeList() []Node {
	nodes := make([]Node, 0, len(c.devices)+len(c.switches))

	for _, d := range c.devices {
		nodes = append(nodes, d)
	}

	for _, s := range c.switches {
		nodes = append(nodes, s)
	}

	return nodes
}
