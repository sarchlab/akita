package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
	"github.com/sarchlab/akita/v5/simulation/queueing"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// Definition declares the switch, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name), passing one port per link in Ports.Port
// and the matching links in Resources.Links; tooling reads the same
// declaration statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq: 1 * timing.GHz,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

// newState creates one port complex per port, sized by the port's link.
func newState(c *Comp) state {
	ports := c.Ports.Port
	links := c.Resources.Links

	if len(links) != len(ports) {
		panic(fmt.Sprintf(
			"switches: %s: %d ports but %d links in Resources.Links",
			c.Name(), len(ports), len(links)))
	}

	state := state{PortComplexes: make([]portComplexState, len(ports))}
	for i, port := range ports {
		state.PortComplexes[i] = newPortComplex(port, links[i])
	}

	return state
}

func newPortComplex(port messaging.Port, link Link) portComplexState {
	name := port.Name()

	return portComplexState{
		LocalPortName:    name,
		RemotePort:       link.Remote,
		NumInputChannel:  link.NumInputChannel,
		NumOutputChannel: link.NumOutputChannel,
		Latency:          link.Latency,
		PipelineWidth:    link.NumInputChannel,
		Pipeline:         queueing.MakePipeline[routedFlit](link.NumInputChannel, link.Latency),

		RouteBuffer: queueing.MakeBuffer[routedFlit](link.NumInputChannel),

		ForwardBuffer: queueing.MakeBuffer[routedFlit](link.NumInputChannel),

		SendOutBuffer: queueing.MakeBuffer[routedFlit](link.NumOutputChannel),
	}
}

// newMiddlewares creates the middlewares. Both share the index from a port,
// local or remote, to its port complex.
func newMiddlewares(c *Comp) middlewares {
	if c.Resources.RoutingTable == nil {
		panic("switches: Resources.RoutingTable is required")
	}

	portIndex := make(map[messaging.RemotePort]int)
	for i, port := range c.Ports.Port {
		portIndex[port.AsRemote()] = i

		if remote := c.Resources.Links[i].Remote; remote != "" {
			portIndex[remote] = i
		}
	}

	return middlewares{
		RouteForwardSend: &routeForwardSendMW{
			comp:         c,
			portIndex:    portIndex,
			routingTable: c.Resources.RoutingTable,
		},
		ReceivePipeline: &receivePipelineMW{
			comp:      c,
			portIndex: portIndex,
		},
	}
}
