package routing

import (
	"sort"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Route is one immutable destination-to-output-port entry in a switch Spec.
type Route struct {
	Destination messaging.RemotePort `json:"destination"`
	Output      messaging.RemotePort `json:"output"`
}

// Table finds next-hop ports and exports configuration for switch Specs.
type Table interface {
	// Snapshot returns independent, deterministically ordered configuration.
	Snapshot() ([]Route, messaging.RemotePort)
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

func (t *table) Snapshot() ([]Route, messaging.RemotePort) {
	routes := make([]Route, 0, len(t.t))
	for dst, output := range t.t {
		routes = append(routes, Route{Destination: dst, Output: output})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Destination < routes[j].Destination })
	return routes, t.defaultPort
}
