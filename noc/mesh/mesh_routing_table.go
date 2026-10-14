package mesh

import (
	"sort"

	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// meshRoutingTable is a routing table that can find the next-hop port according
// to the coordinate of the final destination.
type meshRoutingTable struct {
	x, y, z                               int
	top, left, bottom, right, front, back messaging.RemotePort
	local                                 messaging.RemotePort
	dstTable                              map[messaging.RemotePort]*tile
}

// FindPort finds the next-hop port according to the coordinate of the final
// destination.
func (t *meshRoutingTable) FindPort(dst messaging.RemotePort) messaging.RemotePort {
	dstTile := t.dstTable[dst]
	dstX, dstY, dstZ := dstTile.rt.x, dstTile.rt.y, dstTile.rt.z

	switch {
	case dstZ < t.z:
		return t.front
	case dstZ > t.z:
		return t.back
	case dstY < t.y:
		return t.top
	case dstY > t.y:
		return t.bottom
	case dstX < t.x:
		return t.left
	case dstX > t.x:
		return t.right
	case dstX == t.x && dstY == t.y && dstZ == t.z:
		return t.local
	default:
		panic("unreachable")
	}
}

// DefineRoute does noting
func (t *meshRoutingTable) DefineRoute(finalDst, outputPort messaging.RemotePort) {
	// Do nothing.
}

// DefineDefaultRoute sets the local port.
func (t *meshRoutingTable) DefineDefaultRoute(outputPort messaging.RemotePort) {
	t.local = outputPort
}

// Snapshot materializes the current coordinate routing into immutable Spec data.
func (t *meshRoutingTable) Snapshot() ([]routing.Route, messaging.RemotePort) {
	routes := make([]routing.Route, 0, len(t.dstTable))
	for dst := range t.dstTable {
		routes = append(routes, routing.Route{Destination: dst, Output: t.FindPort(dst)})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Destination < routes[j].Destination })
	return routes, ""
}
