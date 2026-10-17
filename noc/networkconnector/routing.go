package networkconnector

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/noc/internal/endpoint"
	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
)

// Remote records the link between two nodes.
type Remote struct {
	LocalNode Node
	LocalPort messaging.Port

	RemoteNode Node
	RemotePort messaging.Port

	Link *direct.Connection
}

// Bandwidth returns the unlimited bandwidth of an ideal direct link.
func (r Remote) Bandwidth(_ int) float64 {
	return math.Inf(1)
}

// Node represents an endpoint or a switch.
type Node interface {
	ListRemotes() []Remote
	Table() routing.Table
	Name() string
}

// switchNode owns setup-time wiring for a switch built by AddSwitch.
type switchNode struct {
	name         string
	network      *Connector
	table        routing.Table
	ports        []messaging.Port
	ends         []LinkEndSwitchParameter
	sw           *switches.Comp
	remotes      []Remote
	orderedPorts []messaging.Port
}

func (sn *switchNode) ListRemotes() []Remote {
	return sn.remotes
}

func (sn *switchNode) Name() string {
	return sn.name
}

func (sn *switchNode) Table() routing.Table {
	return sn.table
}

// addPort binds immediately, so every address comes from a bound port.
func (sn *switchNode) addPort(param LinkEndSwitchParameter) messaging.Port {
	sn.network.simulation.RequireSetup()
	port := twowaybuffered.NewPort(param.OutgoingBufSize, param.OutgoingBufSize)
	sn.sw.BindPort(fmt.Sprintf("Links[%d]", len(sn.ports)), port)
	sn.ports = append(sn.ports, port)
	sn.ends = append(sn.ends, param)
	return port
}

func (sn *switchNode) Links() []switches.Link {
	ports := sn.Ports()
	links := make([]switches.Link, len(ports))
	for i, port := range ports {
		for _, remote := range sn.remotes {
			if remote.LocalPort == port {
				var p LinkEndSwitchParameter
				for j, bound := range sn.ports {
					if bound == port {
						p = sn.ends[j]
						break
					}
				}
				links[i] = switches.Link{Remote: remote.RemotePort.AsRemote(), Latency: p.Latency,
					NumInputChannel: p.NumInputChannel, NumOutputChannel: p.NumOutputChannel}
				break
			}
		}
	}
	return links
}

func (sn *switchNode) Routes() switches.RoutingTable {
	sn.network.initializeRoutes()
	return sn.network.routes[sn.name]
}

type deviceNode struct {
	ports     []messaging.Port
	parameter DeviceToSwitchLinkParameter
	endPoint  *endpoint.Comp
	sw        *switchNode
	remote    Remote
}

func (dn *deviceNode) ListRemotes() []Remote {
	return []Remote{dn.remote}
}

func (dn *deviceNode) Name() string {
	return dn.endPoint.Name()
}

func (dn *deviceNode) Table() routing.Table {
	return nil
}

// Router computes setup-time routes once during Initialize. The network freezes
// the results; changing the supplied tables later has no runtime effect.
// A router may also implement OrderPorts(Node) []messaging.Port to define a
// deterministic switch arbitration order. It must return every local port once.
type Router interface {
	EstablishRoute(nodes []Node)
}

func (r Remote) localAddress() messaging.RemotePort { return r.LocalPort.AsRemote() }

// Ports returns the runtime arbitration order. A topology-specific router may
// preserve its own order independently of the order links were assembled.
func (sn *switchNode) Ports() []messaging.Port {
	sn.network.initializeRoutes()
	return sn.orderedPorts
}

func (sn *switchNode) orderPorts() []messaging.Port {
	if orderer, ok := sn.network.router.(interface{ OrderPorts(Node) []messaging.Port }); ok {
		ports := orderer.OrderPorts(sn)
		if len(ports) != len(sn.ports) {
			panic("networkconnector: invalid port order")
		}
		remaining := make(map[messaging.Port]bool)
		for _, p := range sn.ports {
			remaining[p] = true
		}
		for _, p := range ports {
			if !remaining[p] {
				panic("networkconnector: invalid port order")
			}
			delete(remaining, p)
		}
		return ports
	}
	return sn.ports
}
