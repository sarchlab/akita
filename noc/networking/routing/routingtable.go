package routing

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// Table is a routing table that can find the next-hop port according to the
// final destination.
type Table interface {
	FindPort(dst messaging.RemotePort) messaging.RemotePort
	DefineRoute(finalDst, outputPort messaging.RemotePort)
	DefineDefaultRoute(outputPort messaging.RemotePort)
}

// NewTable creates a new Table.
func NewTable() Table {
	t := &table{}
	t.t = make(map[messaging.RemotePort]messaging.RemotePort)

	return t
}

type table struct {
	t           map[messaging.RemotePort]messaging.RemotePort
	defaultPort messaging.RemotePort
}

func (t table) FindPort(dst messaging.RemotePort) messaging.RemotePort {
	out, found := t.t[dst]
	if found {
		return out
	}

	return t.defaultPort
}

func (t *table) DefineRoute(finalDst, outputPort messaging.RemotePort) {
	t.t[finalDst] = outputPort
}

func (t *table) DefineDefaultRoute(outputPort messaging.RemotePort) {
	t.defaultPort = outputPort
}
