package routing

import "github.com/sarchlab/akita/v5/sim/messaging"

// Table holds setup-time routes. Routes are computed at Initialize; changing
// a table afterwards is not supported.
type Table struct {
	t           map[messaging.RemotePort]messaging.RemotePort
	defaultPort messaging.RemotePort
}

// NewTable creates a new Table.
func NewTable() *Table {
	t := &Table{}
	t.t = make(map[messaging.RemotePort]messaging.RemotePort)

	return t
}

// FindPort returns the next hop, or the default port if no route matches.
func (t *Table) FindPort(dst messaging.RemotePort) messaging.RemotePort {
	out, found := t.t[dst]
	if found {
		return out
	}

	return t.defaultPort
}

// DefineRoute sets the next hop for a destination during setup.
func (t *Table) DefineRoute(finalDst, outputPort messaging.RemotePort) {
	if t.t == nil {
		t.t = make(map[messaging.RemotePort]messaging.RemotePort)
	}
	t.t[finalDst] = outputPort
}

// DefineDefaultRoute sets the fallback next hop during setup.
func (t *Table) DefineDefaultRoute(outputPort messaging.RemotePort) {
	t.defaultPort = outputPort
}
