package mesh

import (
	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/messaging"
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
