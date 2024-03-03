package networkconnector

import (
	"math"

	"github.com/sarchlab/akita/v4/noc/networking/routing"
	"github.com/sarchlab/akita/v4/noc/networking/switching/endpoint"
	"github.com/sarchlab/akita/v4/noc/networking/switching/switches"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

// Remote records the link between two nodes.
type Remote struct {
	LocalNode Node
	LocalPort sim.Port

	RemoteNode Node
	RemotePort sim.Port

	Link sim.Connection
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

type switchNode struct {
	sw      *switches.Comp
	remotes []Remote
}

func (sn *switchNode) ListRemotes() []Remote {
	return sn.remotes
}

func (sn *switchNode) Name() string {
	return sn.sw.Name()
}

func (sn *switchNode) Table() routing.Table {
	return sn.sw.GetRoutingTable()
}

type deviceNode struct {
	ports    []sim.Port
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
