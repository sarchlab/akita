package networkconnector

import (
	"fmt"

	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/noc/networking/switching/endpoint"
	"github.com/sarchlab/akita/v5/noc/networking/switching/switches"

	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/timing"
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

// PortFactory creates a port with the given full name and buffer capacities,
// like messaging.NewPort, the default. The port has no owner yet; the Build of
// the component it is given to binds it.
type PortFactory func(name string, incomingBufCap, outgoingBufCap int) messaging.Port

// Connector can build complex network topologies.
type Connector struct {
	name        string
	engine      timing.EventScheduler
	simulation  timing.Simulation
	defaultFreq timing.Freq
	flitSize    int
	router      Router
	portFactory PortFactory

	switches        []*switchNode
	devices         []*deviceNode
	connectionCount int
}

// MakeConnector creates a network connector
func MakeConnector() Connector {
	return Connector{
		defaultFreq: 1 * timing.GHz,
		flitSize:    64,
		router:      new(FloydWarshallRouter),
		portFactory: messaging.NewPort,
	}
}

// WithSimulation sets the simulation that owns the network components.
func (c Connector) WithSimulation(sim timing.Simulation) Connector {
	c.simulation = sim
	c.engine = sim.Engine()
	return c
}

// WithDefaultFreq sets the default frequency used by the components in the
// connection. Note that channels will not use the default frequency. Channels
// use their own frequency to adjust bandwidth.
func (c Connector) WithDefaultFreq(f timing.Freq) Connector {
	c.defaultFreq = f
	return c
}

// WithFlitSize sets the flit size to be used throughout the network.
func (c Connector) WithFlitSize(size int) Connector {
	c.flitSize = size
	return c
}

// WithRouter sets the router to use to establish the routing tables.
func (c Connector) WithRouter(r Router) Connector {
	c.router = r
	return c
}

// WithPortFactory sets the factory function used to create ports.
func (c Connector) WithPortFactory(f PortFactory) Connector {
	c.portFactory = f
	return c
}

// NewNetwork resets the connector, making it ready to create a new network
// with the give Name.
func (c *Connector) NewNetwork(name string) {
	c.name = name
	c.switches = nil
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

	epPort = c.portFactory(epFullName+".NetworkPort",
		param.DeviceEndParam.IncomingBufSize,
		param.DeviceEndParam.OutgoingBufSize)
	swPort, _ = swNode.addPort(c.portFactory, epPort.AsRemote(),
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
	epSpec.DefaultSwitchDst = swPort.AsRemote()

	endPoint := endpoint.Definition.Builder().
		WithSimulation(c.simulation).
		WithSpec(epSpec).
		WithResources(endpoint.Resources{DevicePorts: ports}).
		WithPorts(endpoint.Ports{NetworkPort: epPort}).
		Build(name)

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
	conn messaging.Connection,
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
) (conn messaging.Connection) {
	connName := fmt.Sprintf("%s.Conn[%d]", c.name, c.connectionCount)
	c.connectionCount++

	if linkParam.IsIdeal {
		conn = directconnection.MakeBuilder().
			WithSimulation(c.simulation).
			WithSpec(directconnection.Spec{Freq: c.defaultFreq}).
			Build(connName)
	} else {
		panic("non-ideal (with latency) connection is not implemented.")
	}

	conn.PlugIn(left)
	conn.PlugIn(right)

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
	leftPort, leftLink := leftNode.addPort(c.portFactory, "", param.LeftEndParam)
	rightPort, rightLink := rightNode.addPort(c.portFactory, "", param.RightEndParam)
	leftNode.links[leftLink].Remote = rightPort.AsRemote()
	rightNode.links[rightLink].Remote = leftPort.AsRemote()

	conn := c.connectPorts(leftPort, rightPort, param.LinkParam)

	c.createRemoteInfo(leftNode, rightNode, leftPort, rightPort, conn)

	return leftPort, rightPort
}

func (c *Connector) createRemoteInfo(
	leftNode, rightNode *switchNode,
	leftPort, rightPort messaging.Port,
	conn messaging.Connection,
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
// It first builds the switches: a switch takes all of its ports at Build, so
// the switches are built once every device and link is connected. Call it
// after the last ConnectDevice and ConnectSwitches of the network.
func (c *Connector) EstablishRoute() {
	c.BuildSwitches()

	if c.router == nil {
		return
	}

	nodes := c.createRoutingNodeList()
	c.router.EstablishRoute(nodes)
}

// BuildSwitches builds every switch that is not built yet, with the ports and
// links added to it. EstablishRoute calls it; a connector that fills the
// routing tables itself calls it once every device and link is connected.
func (c *Connector) BuildSwitches() {
	for _, node := range c.switches {
		if node.sw != nil {
			continue
		}

		node.sw = switches.Definition.Builder().
			WithSimulation(c.simulation).
			WithSpec(node.spec).
			WithResources(switches.Resources{
				RoutingTable: node.table,
				Links:        node.links,
			}).
			WithPorts(switches.Ports{Port: node.ports}).
			Build(node.name)
	}
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
