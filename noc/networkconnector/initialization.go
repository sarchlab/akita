package networkconnector

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// frozenRoutes is never returned to the setup router or modified after Initialize.
type frozenRoutes map[messaging.RemotePort]messaging.RemotePort

func (r frozenRoutes) FindPort(dst messaging.RemotePort) messaging.RemotePort { return r[dst] }

type connectionRecord struct {
	Name        string
	Left, Right messaging.RemotePort
	Parameter   LinkParameter
}

func (c *Connector) initializeRoutes() {
	if c.routes != nil {
		return
	}
	nodes := c.createRoutingNodeList()
	c.validateConnectivity(nodes)
	if c.router != nil {
		c.router.EstablishRoute(nodes)
	}
	routes := make(map[string]frozenRoutes, len(c.switches))
	for _, sw := range c.switches {
		table := make(frozenRoutes)
		for _, device := range c.devices {
			for _, dst := range device.ports {
				output := sw.table.FindPort(dst.AsRemote())
				valid := false
				for _, remote := range sw.remotes {
					if output == remote.LocalPort.AsRemote() {
						valid = true
						break
					}
				}
				if !valid {
					panic(fmt.Sprintf("%s: no valid route from %s to %s", c.name, sw.name, dst.AsRemote()))
				}
				table[dst.AsRemote()] = output
			}
		}
		routes[sw.name] = table
	}
	// Follow every forwarding path, not just its first hop: custom tables can
	// otherwise contain loops or send a destination to the wrong endpoint.
	for _, sw := range c.switches {
		for _, device := range c.devices {
			for _, dst := range device.ports {
				c.validatePath(sw, device, dst.AsRemote(), routes)
			}
		}
	}
	for _, sw := range c.switches {
		sw.orderedPorts = append([]messaging.Port(nil), sw.orderPorts()...)
	}
	c.routes = routes
}

func (c *Connector) validatePath(
	start *switchNode, dst *deviceNode, address messaging.RemotePort, routes map[string]frozenRoutes,
) {
	current := Node(start)
	seen := make(map[Node]bool)
	for current != dst {
		if seen[current] {
			panic(fmt.Sprintf("%s: routing loop from %s to %s", c.name, start.name, address))
		}
		seen[current] = true
		sw, ok := current.(*switchNode)
		if !ok {
			panic(fmt.Sprintf("%s: route to %s reaches the wrong endpoint", c.name, address))
		}
		next := routes[sw.name][address]
		var remoteNode Node
		for _, remote := range sw.remotes {
			if remote.LocalPort.AsRemote() == next {
				remoteNode = remote.RemoteNode
				break
			}
		}
		if remoteNode == nil {
			panic(fmt.Sprintf("%s: unreachable destination %s", c.name, address))
		}
		current = remoteNode
	}
}

// The resource is registered before its switches. Switch requests also use the
// same cache, so computing routes does not depend on component initialization order.
type networkCheckpoint struct{ c *Connector }

func (n *networkCheckpoint) Name() string         { return n.c.name + ".Topology" }
func (n *networkCheckpoint) ValidateSetup() error { return nil }
func (n *networkCheckpoint) InitializeComponent() { n.c.initializeRoutes() }

type switchRecord struct {
	Name      string
	TableType string
	Ports     []messaging.RemotePort
	Links     []switches.Link
	Ends      []LinkEndSwitchParameter
	Routes    frozenRoutes
}
type deviceRecord struct {
	Name      string
	Ports     []messaging.RemotePort
	Parameter DeviceToSwitchLinkParameter
}
type networkRecord struct {
	Router      string
	Switches    []switchRecord
	Devices     []deviceRecord
	Connections []connectionRecord
}

func (n *networkCheckpoint) record() networkRecord {
	c := n.c
	c.initializeRoutes()
	record := networkRecord{Router: fmt.Sprintf("%T", c.router), Connections: c.connections}
	for _, sw := range c.switches {
		item := switchRecord{Name: sw.name,
			TableType: fmt.Sprintf("%T", sw.table),
			Links:     sw.Links(),
			Ends:      sw.ends,
			Routes:    c.routes[sw.name]}
		for _, p := range sw.Ports() {
			item.Ports = append(item.Ports, p.AsRemote())
		}
		record.Switches = append(record.Switches, item)
	}
	for _, device := range c.devices {
		item := deviceRecord{Name: device.endPoint.Name(), Parameter: device.parameter}
		for _, port := range device.ports {
			item.Ports = append(item.Ports, port.AsRemote())
		}
		record.Devices = append(record.Devices, item)
	}
	return record
}
func (n *networkCheckpoint) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, n.record(), struct{}{}, nil)
}
func (n *networkCheckpoint) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, n.record(), &struct{}{}, nil)
}

func (c *Connector) validateConnectivity(nodes []Node) {
	if len(nodes) == 0 {
		return
	}
	seen := map[Node]bool{nodes[0]: true}
	pending := []Node{nodes[0]}
	for len(pending) > 0 {
		node := pending[0]
		pending = pending[1:]
		for _, remote := range node.ListRemotes() {
			if remote.LocalPort.Connection() != remote.Link || remote.RemotePort.Connection() != remote.Link {
				panic(fmt.Sprintf("%s: inconsistent link at %s", c.name, node.Name()))
			}
			if !seen[remote.RemoteNode] {
				seen[remote.RemoteNode] = true
				pending = append(pending, remote.RemoteNode)
			}
		}
	}
	for _, node := range nodes {
		if !seen[node] {
			panic(fmt.Sprintf("%s: disconnected node %s", c.name, node.Name()))
		}
	}
}
