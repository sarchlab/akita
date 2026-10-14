package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/noc/routing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the switch, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name), then binds one port per link in Ports.Links
// and the matching links in Spec.Links; tooling reads the same
// declaration statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq: 1 * timing.GHz,
	},
	ValidateSpec:   validateSpec,
	ValidateSetup:  validateSetup,
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

// newState creates one port complex per port, sized by the port's link.
func newState(c *Comp) state {
	ports := c.Ports.Links
	links := c.Spec.Links

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
	table := routing.NewTable()
	for _, route := range c.Spec.Routes {
		table.DefineRoute(route.Destination, route.Output)
	}
	table.DefineDefaultRoute(c.Spec.DefaultRoute)

	portIndex := make(map[messaging.RemotePort]int)
	for i, port := range c.Ports.Links {
		portIndex[port.AsRemote()] = i

		if remote := c.Spec.Links[i].Remote; remote != "" {
			portIndex[remote] = i
		}
	}

	return middlewares{
		RouteForwardSend: &routeForwardSendMW{
			comp:         c,
			portIndex:    portIndex,
			routingTable: table,
		},
		ReceivePipeline: &receivePipelineMW{
			comp:      c,
			portIndex: portIndex,
		},
	}
}

func validateSpec(spec Spec) error {
	for i, link := range spec.Links {
		if link.Remote == "" || link.Latency < 0 || link.NumInputChannel <= 0 || link.NumOutputChannel <= 0 {
			return fmt.Errorf("switches: link %d needs a remote, non-negative latency and positive channel counts", i)
		}
	}
	seen := map[messaging.RemotePort]bool{}
	for _, route := range spec.Routes {
		if route.Destination == "" || route.Output == "" || seen[route.Destination] {
			return fmt.Errorf("switches: invalid or duplicate route for %q", route.Destination)
		}
		seen[route.Destination] = true
	}
	return nil
}

func validateSetup(c *Comp) error {
	if len(c.Spec.Links) != len(c.Ports.Links) {
		return fmt.Errorf("switches: %d ports but %d links in Spec.Links", len(c.Ports.Links), len(c.Spec.Links))
	}
	return nil
}
