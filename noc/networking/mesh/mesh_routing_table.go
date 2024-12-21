package mesh

import "github.com/sarchlab/akita/v4/sim/modeling"

// meshRoutingTable is a routing table that can find the next-hop port according
// to the coordinate of the final destination.
type meshRoutingTable struct {
	x, y, z                               int
	top, left, bottom, right, front, back modeling.RemotePort
	local                                 modeling.RemotePort
	dstTable                              map[modeling.RemotePort]*tile
}

// FindPort finds the next-hop port according to the coordinate of the final
// destination.
func (t *meshRoutingTable) FindPort(
	dst modeling.RemotePort,
) modeling.RemotePort {
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
func (t *meshRoutingTable) DefineRoute(
	finalDst, outputPort modeling.RemotePort,
) {
	// Do nothing.
}

// DefineDefaultRoute sets the local port.
func (t *meshRoutingTable) DefineDefaultRoute(
	outputPort modeling.RemotePort,
) {
	t.local = outputPort
}
