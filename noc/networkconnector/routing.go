package networkconnector

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/noc/internal/endpoint"
	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
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
	Table() *routing.Table
	Name() string
}

// switchNode owns setup-time wiring for a switch built by AddSwitch.
type switchNode struct {
	name    string
	network *Connector
	table   *routing.Table
	ports   []messaging.Port
	ends    []LinkEndSwitchParameter
	sw      *switches.Comp
	remotes []Remote
}

func (sn *switchNode) ListRemotes() []Remote {
	return sn.remotes
}

func (sn *switchNode) Name() string {
	return sn.name
}

func (sn *switchNode) Table() *routing.Table {
	return sn.table
}

// addPort binds immediately, so every address comes from a bound port.
func (sn *switchNode) addPort(param LinkEndSwitchParameter) messaging.Port {
	assembly.RequireSetup(sn.network.simulation)
	port := twowaybuffered.MakeBuilder().
		WithSimulation(sn.network.simulation).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: param.IncomingBufSize, OutgoingCapacity: param.OutgoingBufSize}).
		Build()
	sn.sw.BindPort(fmt.Sprintf("Links[%d]", len(sn.ports)), port)
	sn.ports = append(sn.ports, port)
	sn.ends = append(sn.ends, param)
	return port
}

func (sn *switchNode) Links() []switches.Link {
	links := make([]switches.Link, len(sn.ports))
	for i, remote := range sn.remotes {
		p := sn.ends[i]
		links[i] = switches.Link{Remote: remote.RemotePort.AsRemote(), Latency: p.Latency,
			NumInputChannel: p.NumInputChannel, NumOutputChannel: p.NumOutputChannel}
	}
	return links
}

func (sn *switchNode) Routes() switches.RoutingTable { return sn.network.routes[sn.name] }

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

func (dn *deviceNode) Table() *routing.Table {
	return nil
}

// Router computes setup-time routes during Initialize. Changing tables after
// initialization is not supported.
type Router interface{ EstablishRoute(nodes []Node) }

func (r Remote) localAddress() messaging.RemotePort { return r.LocalPort.AsRemote() }
