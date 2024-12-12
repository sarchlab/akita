package routing

import "github.com/sarchlab/akita/v4/sim/modeling"

// Table is a routing table that can find the next-hop port according to the
// final destination.
type Table interface {
	FindPort(dst modeling.RemotePort) modeling.RemotePort
	DefineRoute(finalDst, outputPort modeling.RemotePort)
	DefineDefaultRoute(outputPort modeling.RemotePort)
}

// NewTable creates a new Table.
func NewTable() Table {
	t := &table{}
	t.t = make(map[modeling.RemotePort]modeling.RemotePort)

	return t
}

type table struct {
	t           map[modeling.RemotePort]modeling.RemotePort
	defaultPort modeling.RemotePort
}

func (t table) FindPort(dst modeling.RemotePort) modeling.RemotePort {
	out, found := t.t[dst]
	if found {
		return out
	}

	return t.defaultPort
}

func (t *table) DefineRoute(finalDst, outputPort modeling.RemotePort) {
	t.t[finalDst] = outputPort
}

func (t *table) DefineDefaultRoute(outputPort modeling.RemotePort) {
	t.defaultPort = outputPort
}
