package networkconnector

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/noc/networking/switching/endpoint"
	"github.com/sarchlab/akita/v5/noc/networking/switching/switches"

	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation/messaging"
)

// Remote records the link between two nodes.
type Remote struct {
	LocalNode Node
	LocalPort messaging.Port

	RemoteNode Node
	RemotePort messaging.Port

	Link messaging.Connection
}

// Bandwidth returns the bandwidth of the link.
func (r Remote) Bandwidth(flitSize int) float64 {
	switch r.Link.(type) {
	case *directconnection.Comp:
		return math.Inf(1)
	// case *messaging.Channel:
	// 	return float64(l.Freq) * float64(flitSize)
	default:
		panic("unknown link type")
	}
}

// Node represents an endpoint or a switch.
type Node interface {
	ListRemotes() []Remote
	Table() routing.Table
	Name() string
}

// switchNode is a switch of the network. Its ports and links are collected as
// devices and switches are connected; the switch itself is built from them by
// EstablishRoute.
type switchNode struct {
	name    string
	spec    switches.Spec
	table   routing.Table
	ports   []messaging.Port
	links   []switches.Link
	sw      *switches.Comp
	remotes []Remote
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

// addPort creates the switch's next port and the link behind it, and returns
// the port and the link's index. remote may be empty and set later, once the
// port at the other end exists.
func (sn *switchNode) addPort(
	factory PortFactory,
	remote messaging.RemotePort,
	param LinkEndSwitchParameter,
) (messaging.Port, int) {
	if sn.sw != nil {
		panic(fmt.Sprintf("networkconnector: switch %s is already built", sn.name))
	}

	port := factory(fmt.Sprintf("%s.Port[%d]", sn.name, len(sn.ports)),
		param.OutgoingBufSize, param.OutgoingBufSize)

	sn.ports = append(sn.ports, port)
	sn.links = append(sn.links, switches.Link{
		Remote:           remote,
		Latency:          param.Latency,
		NumInputChannel:  param.NumInputChannel,
		NumOutputChannel: param.NumOutputChannel,
	})

	return port, len(sn.links) - 1
}

type deviceNode struct {
	ports    []messaging.Port
	endPoint *endpoint.Comp
	sw       *switchNode
	remote   Remote
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

// Router can help establish the routes of a network.
type Router interface {
	EstablishRoute(nodes []Node)
}
