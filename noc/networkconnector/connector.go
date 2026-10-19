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
// connected to a switch.
type LinkEndSwitchParameter struct {
	IncomingBufSize  int
	OutgoingBufSize  int
	NumInputChannel  int
	NumOutputChannel int
	Latency          int
}

// LinkEndDeviceParameter defines the parameter that associated with an end of a
// link that is connected to a device.
type LinkEndDeviceParameter struct {
	IncomingBufSize  int
	OutgoingBufSize  int
	NumInputChannel  int
	NumOutputChannel int
}

// LinkParameter defines the parameter of the link that connects to nodes.
type LinkParameter struct {
	IsIdeal       bool
	Frequency     timing.Freq
	NumStage      int
	CyclePerStage int
	PipelineWidth int
}

// DeviceToSwitchLinkParameter contains the parameters that define a link
// between a device and a switch.
type DeviceToSwitchLinkParameter struct {
	DeviceEndParam LinkEndDeviceParameter
	SwitchEndParam LinkEndSwitchParameter
	LinkParam      LinkParameter
}

// SwitchToSwitchLinkParameter contains the parameters that define a link
// between two switches.
type SwitchToSwitchLinkParameter struct {
	LeftEndParam  LinkEndSwitchParameter
	RightEndParam LinkEndSwitchParameter
	LinkParam     LinkParameter
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
	connections     []connectionRecord
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
	b.simulation.RegisterResource(&networkCheckpoint{
		Member: assembly.NewMember(b.simulation, c.initializeRoutes, nil), c: c,
	})
	return c
}

// AddSwitch adds a new switch to the network.
func (c *Connector) AddSwitch() (switchID int) {
	switchID = len(c.switches)
	name := fmt.Sprintf("Switch[%d]", switchID)

	c.AddSwitchWithName(name)

	return switchID
}

// AddSwitchWithNameAndRoutingTable adds a new switch to the network with an
// externally provided name and routing table.
func (c *Connector) AddSwitchWithNameAndRoutingTable(
	swName string,
	rt *routing.Table,
) (switchID int) {
	assembly.RequireSetup(c.simulation)
	switchID = len(c.switches)

	spec := switches.DefaultSpec()
	spec.Freq = c.defaultFreq

	node := &switchNode{
		name:  fmt.Sprintf("%s.%s", c.name, swName),
		table: rt, network: c,
	}
	node.sw = modeling.MakeBuilder[*switches.Comp](spec).
		WithSimulation(c.simulation).
		WithResources(switches.Resources{Wiring: node}).Build(node.name)
	c.switches = append(c.switches, node)

	return switchID
}

// AddSwitchWithName adds a new switch to the network with an externally
// provided Name.
func (c *Connector) AddSwitchWithName(swName string) (switchID int) {
	routingTable := routing.NewTable()
	return c.AddSwitchWithNameAndRoutingTable(swName, routingTable)
}

// ConnectDevice connects a few ports that belongs to the device to a switch
// that is identified by switchID.
func (c *Connector) ConnectDevice(
	switchID int,
	ports []messaging.Port,
	param DeviceToSwitchLinkParameter,
) {
	name := fmt.Sprintf("EndPoint[%d]", len(c.devices))
	c.ConnectDeviceWithEPName(name, switchID, ports, param)
}

// ConnectDeviceWithEPName connects a few ports that belongs to the device to a
// switch that is identified by switchID, through an endpoint with the given
// name. Repeated calls with the same name, switch and parameters append device
// ports to that endpoint in call order; this supports incremental mesh tiles.
func (c *Connector) ConnectDeviceWithEPName(
	epName string,
	switchID int,
	ports []messaging.Port,
	param DeviceToSwitchLinkParameter,
) {
	assembly.RequireSetup(c.simulation)
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
			IncomingCapacity: param.DeviceEndParam.IncomingBufSize,
			OutgoingCapacity: param.DeviceEndParam.OutgoingBufSize,
		}).
		Build()
	swPort := swNode.addPort(param.SwitchEndParam)

	epNode := c.createEndPoint(epFullName, ports, param, swNode, epPort, swPort)
	conn := c.connectPorts(epPort, swPort, param.LinkParam)
	c.createRemoteInfoFoEP(epNode, swNode, epPort, swPort, conn)
}

// createEndPoint builds the endpoint that carries the device ports' messages
// over the link from epPort to swPort.
func (c *Connector) createEndPoint(
	name string,
	ports []messaging.Port,
	param DeviceToSwitchLinkParameter,
	swNode *switchNode,
	epPort, swPort messaging.Port,
) *deviceNode {
	epSpec := endpoint.DefaultSpec()
	epSpec.Freq = c.defaultFreq
	epSpec.FlitByteSize = c.flitSize
	epSpec.NumInputChannels = param.DeviceEndParam.NumInputChannel
	epSpec.NumOutputChannels = param.DeviceEndParam.NumOutputChannel
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
	linkParam LinkParameter,
) (conn *direct.Connection) {
	connName := fmt.Sprintf("%s.Conn[%d]", c.name, c.connectionCount)
	c.connectionCount++

	if linkParam.IsIdeal {
		conn = direct.NewConnection(connName, c.simulation, c.defaultFreq)
	} else {
		panic("non-ideal (with latency) connection is not implemented.")
	}

	conn.BindPort(left)
	conn.BindPort(right)
	c.connections = append(c.connections, connectionRecord{conn.Name(), left.AsRemote(), right.AsRemote(), linkParam})

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

	conn := c.connectPorts(leftPort, rightPort, param.LinkParam)

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
