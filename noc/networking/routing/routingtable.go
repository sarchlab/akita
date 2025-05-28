package routing

import "github.com/sarchlab/akita/v4/sim"

// Table is a routing table that can find the next-hop port according to the
// final destination.
type Table interface {
	FindPort(dst sim.RemotePort) sim.RemotePort
	DefineRoute(finalDst, outputPort sim.RemotePort)
	DefineDefaultRoute(outputPort sim.RemotePort)
}

// NewTable creates a new Table.
func NewTable() Table {
	t := &table{}
	t.t = make(map[sim.RemotePort]sim.RemotePort)

	return t
}

type table struct {
	t           map[sim.RemotePort]sim.RemotePort
	defaultPort sim.RemotePort
}

func (t table) FindPort(dst sim.RemotePort) sim.RemotePort {
	out, found := t.t[dst]
	if found {
		return out
	}

	return t.defaultPort
}

func (t *table) DefineRoute(finalDst, outputPort sim.RemotePort) {
	t.t[finalDst] = outputPort
}

func (t *table) DefineDefaultRoute(outputPort sim.RemotePort) {
	t.defaultPort = outputPort
}
