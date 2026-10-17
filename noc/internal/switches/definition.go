package switches

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq: 1 * timing.GHz,
	}
}

// Initialize creates one port complex per port, sized by the port's link.
func (s *state) Initialize(c *Comp) {
	if c.Resources.Wiring == nil {
		panic(c.Name() + ": switches: network wiring is required")
	}
	ports := c.Resources.Wiring.Ports()
	links := c.Resources.Wiring.Links()
	if len(links) != len(ports) {
		panic(fmt.Sprintf("%s: switches: %d ports but %d link settings", c.Name(), len(ports), len(links)))
	}
	for i, link := range links {
		if link.Remote == "" || link.Latency < 0 || link.NumInputChannel <= 0 || link.NumOutputChannel <= 0 {
			panic(fmt.Sprintf("%s: switches: invalid link %d", c.Name(), i))
		}
	}

	state := state{PortComplexes: make([]portComplexState, len(ports))}
	for i, port := range ports {
		state.PortComplexes[i] = newPortComplex(port, links[i])
	}

	*s = state
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

// Initialize creates the middlewares. Both share the index from a port,
// local or remote, to its port complex.
func (m *middlewares) Initialize(c *Comp) {
	table := c.Resources.Wiring.Routes()
	links := c.Resources.Wiring.Links()

	portIndex := make(map[messaging.RemotePort]int)
	ports := c.Resources.Wiring.Ports()
	for i, port := range ports {
		portIndex[port.AsRemote()] = i

		if remote := links[i].Remote; remote != "" {
			portIndex[remote] = i
		}
	}

	*m = middlewares{
		RouteForwardSend: &routeForwardSendMW{
			comp:         c,
			orderedPorts: ports,
			portIndex:    portIndex,
			routingTable: table,
		},
		ReceivePipeline: &receivePipelineMW{
			comp:         c,
			orderedPorts: ports,
			portIndex:    portIndex,
		},
	}
}
