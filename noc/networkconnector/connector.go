package networkconnector

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/naming"

	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/noc/switching/endpoint"
	"github.com/sarchlab/akita/v5/noc/switching/switches"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
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
	PortName         string
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
	engine      timing.EventScheduler
	simulation  timing.Simulation
	defaultFreq timing.Freq
	flitSize    int
	router      Router

	switches        []*switchNode
	devices         []*deviceNode
	connectionCount int
	pending         []pendingConnection
}

type pendingConnection struct {
	conn        *direct.Connection
	left, right messaging.Port
}

// Builder configures independent network assembly instances.
type Builder struct {
	simulation timing.Simulation
	freq       timing.Freq
	flitSize   int
	router     Router
}

func MakeBuilder() Builder {
	return Builder{freq: timing.GHz, flitSize: 64, router: FloydWarshallRouter{}}
}
func (b Builder) WithSimulation(s timing.Simulation) Builder { b.simulation = s; return b }
func (b Builder) WithFreq(f timing.Freq) Builder             { b.freq = f; return b }
func (b Builder) WithFlitSize(size int) Builder              { b.flitSize = size; return b }
func (b Builder) WithRouter(router Router) Builder           { b.router = router; return b }

// Build begins an independent network with the supplied name.
func (b Builder) Build(name string) *Connector {
	naming.MustBeValid(name)
	if b.simulation == nil {
		panic("networkconnector: WithSimulation is required")
	}
	b.simulation.RequireSetup()
	if b.freq <= 0 || b.flitSize <= 0 {
		panic("networkconnector: frequency and flit size must be positive")
	}
	return &Connector{
		name: name, simulation: b.simulation, engine: b.simulation.Engine(),
		defaultFreq: b.freq, flitSize: b.flitSize, router: b.router,
	}
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
	rt routing.Table,
) (switchID int) {
	switchID = len(c.switches)

	spec := switches.Definition.DefaultSpec
	spec.Freq = c.defaultFreq

	c.switches = append(c.switches, &switchNode{
		name:  fmt.Sprintf("%s.%s", c.name, swName),
		spec:  spec,
		table: rt,
	})

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
// name. It returns the endpoint's network port and the switch port at the
// other end of the link.
func (c *Connector) ConnectDeviceWithEPName(
	epName string,
	switchID int,
	ports []messaging.Port,
	param DeviceToSwitchLinkParameter,
) (epPort, swPort messaging.Port) {
	swNode := c.switches[switchID]
	epFullName := fmt.Sprintf("%s.%s", c.name, epName)

	epPort = twowaybuffered.NewPort(param.DeviceEndParam.IncomingBufSize,
		param.DeviceEndParam.OutgoingBufSize)
	swPort, _ = swNode.addPort(messaging.RemotePort(epFullName+".NetworkPort"),
		param.SwitchEndParam)

	epNode := c.createEndPoint(epFullName, ports, param, swNode, epPort, swPort)
	conn := c.connectPorts(epPort, swPort, param.LinkParam)
	c.createRemoteInfoFoEP(epNode, swNode, epPort, swPort, conn)

	return epPort, swPort
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
	epSpec := endpoint.Definition.DefaultSpec
	epSpec.Freq = c.defaultFreq
	epSpec.FlitByteSize = c.flitSize
	epSpec.NumInputChannels = param.DeviceEndParam.NumInputChannel
	epSpec.NumOutputChannels = param.DeviceEndParam.NumOutputChannel
	epSpec.DefaultSwitchDst = messaging.RemotePort(fmt.Sprintf("%s.Links[%d]", swNode.name, len(swNode.ports)-1))

	endPoint := endpoint.Definition.Builder().
		WithSimulation(c.simulation).
		WithSpec(epSpec).
		WithResources(endpoint.Resources{DevicePorts: ports}).
		Build(name)

	endPoint.BindPort("NetworkPort", epPort)
	endpoint.ConnectDevices(endPoint)

	epNode := &deviceNode{
		ports:    ports,
		endPoint: endPoint,
		sw:       swNode,
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

	c.pending = append(c.pending, pendingConnection{conn, left, right})

	return conn
}

// ConnectSwitches create a connection between two switches. The connection
// created is bi-directional.
func (c *Connector) ConnectSwitches(
	leftSwitchID, rightSwitchID int,
	param SwitchToSwitchLinkParameter,
) (leftPort, rightPort messaging.Port) {
	leftNode := c.switches[leftSwitchID]
	rightNode := c.switches[rightSwitchID]

	// Each side's link names the other side's port, so add both ports first
	// and then record each remote.
	leftPort, leftLink := leftNode.addPort("", param.LeftEndParam)
	rightPort, rightLink := rightNode.addPort("", param.RightEndParam)
	leftNode.links[leftLink].Remote = messaging.RemotePort(fmt.Sprintf("%s.Links[%d]", rightNode.name, rightLink))
	rightNode.links[rightLink].Remote = messaging.RemotePort(fmt.Sprintf("%s.Links[%d]", leftNode.name, leftLink))

	conn := c.connectPorts(leftPort, rightPort, param.LinkParam)

	c.createRemoteInfo(leftNode, rightNode, leftPort, rightPort, conn)

	return leftPort, rightPort
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

// EstablishRoute sets the routing table for all the nodes.
//
// It computes routes, snapshots them into Spec, then builds and binds switches. Call it
// after the last ConnectDevice and ConnectSwitches of the network.
func (c *Connector) EstablishRoute() {
	if c.router != nil {
		c.router.EstablishRoute(c.createRoutingNodeList())
	}
	c.BuildSwitches()
}

// BuildSwitches builds every switch that is not built yet, with the ports and
// links added to it. EstablishRoute calls it; a connector that fills the
// routing tables itself calls it once every device and link is connected.
func (c *Connector) BuildSwitches() {
	for _, node := range c.switches {
		if node.sw != nil {
			continue
		}

		node.spec.Links = node.links
		node.spec.Routes, node.spec.DefaultRoute = node.table.Snapshot()
		node.sw = switches.Definition.Builder().
			WithSimulation(c.simulation).
			WithSpec(node.spec).
			Build(node.name)
		bindingPorts := switches.Ports{Links: node.ports}
		for i, p := range bindingPorts.Links {
			node.sw.BindPort(fmt.Sprintf("Links[%d]", i), p)
		}
	}
	for _, link := range c.pending {
		link.conn.BindPort(link.left)
		link.conn.BindPort(link.right)
	}

	c.pending = nil
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

// func (c *Connector) dumpRoute() {
// 	fmt.Println("")
// 	for _, swNode := range c.switches {
// 		for _, epNode := range c.devices {
// 			for _, port := range epNode.ports {
// 				nextHopPort := swNode.sw.GetRoutingTable().FindPort(port)

// 				var nextHop Node
// 				for _, remote := range swNode.remotes {
// 					if remote.LocalPort == nextHopPort {
// 						nextHop = remote.RemoteNode
// 					}
// 				}

// 				fmt.Printf("%s -> %s -> %s --- %s\n",
// 					swNode.Name(),
// 					nextHopPort.Name(),
// 					nextHop.Name(),
// 					port.Name())
// 			}
// 		}
// 	}
// }

// PortAddress returns the deterministic address of a port during assembly,
// including switch ports that have not been bound to their component yet.
func (c *Connector) PortAddress(port messaging.Port) messaging.RemotePort {
	for _, sw := range c.switches {
		for _, p := range sw.ports {
			if p == port {
				return sw.portAddress(port)
			}
		}
	}
	return port.AsRemote()
}
