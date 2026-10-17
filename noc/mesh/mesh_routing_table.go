package mesh

import (
	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"sort"
)

// meshRouter derives Z, then Y, then X dimension-order routes from bound ports.
type meshRouter struct{ c *Connector }

func (r meshRouter) EstablishRoute(nodes []networkconnector.Node) {
	byName := make(map[string]networkconnector.Node)
	for _, n := range nodes {
		byName[n.Name()] = n
	}
	r.c.eachLocation(func(src [3]int) {
		t := r.c.tiles[src]
		node := byName[t.switchName]
		outputs := make(map[string]messaging.RemotePort)
		for _, remote := range node.ListRemotes() {
			outputs[remote.RemoteNode.Name()] = remote.LocalPort.AsRemote()
		}
		r.c.eachLocation(func(dst [3]int) {
			target := r.c.tiles[dst]
			next := src
			for axis := 2; axis >= 0; axis-- {
				if dst[axis] < src[axis] {
					next[axis]--
					break
				}
				if dst[axis] > src[axis] {
					next[axis]++
					break
				}
			}
			name := r.c.name + "." + target.endpointName
			if next != src {
				name = r.c.tiles[next].switchName
			}
			for _, port := range target.ports {
				node.Table().DefineRoute(port.AsRemote(), outputs[name])
			}
		})
	})
}

// OrderPorts preserves the old full-grid construction's arbitration order even
// when AddTile grows dimensions in a different order: local, front, top, left,
// back, bottom, right. Binding names remain the actual names assigned at setup.
func (r meshRouter) OrderPorts(node networkconnector.Node) []messaging.Port {
	locations := make(map[string][3]int)
	for _, t := range r.c.tiles {
		locations[t.switchName] = t.location
	}
	here := locations[node.Name()]
	remotes := append([]networkconnector.Remote(nil), node.ListRemotes()...)
	rank := func(remote networkconnector.Remote) int {
		at, isSwitch := locations[remote.RemoteNode.Name()]
		if !isSwitch {
			return 0
		}
		for axis := 2; axis >= 0; axis-- {
			if at[axis] < here[axis] {
				return 3 - axis
			}
			if at[axis] > here[axis] {
				return 6 - axis
			}
		}
		panic("mesh: invalid neighbor")
	}
	sort.Slice(remotes, func(i, j int) bool { return rank(remotes[i]) < rank(remotes[j]) })
	ports := make([]messaging.Port, len(remotes))
	for i, remote := range remotes {
		ports[i] = remote.LocalPort
	}
	return ports
}
