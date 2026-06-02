package switches

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for switch components.
var defaultSpec = Spec{
	Freq: 1 * timing.GHz,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds switches. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithRegistrar and WithResources. Ports
// are added after build with MakeSwitchPortAdder.
type Builder struct {
	registrar modeling.Registrar
	spec      Spec
	resources Resources
}

// MakeBuilder creates a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: defaultSpec,
	}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the external wiring, namely the routing table the
// switch uses to resolve flit destinations.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build creates a new switch
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("switches: WithRegistrar is required")
	}

	b.routingTableMustBeGiven()

	spec := b.spec
	engine := b.registrar.GetEngine()
	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	portIndex := make(map[messaging.RemotePort]int)

	rfsMW := &routeForwardSendMW{
		comp:         modelComp,
		portIndex:    portIndex,
		routingTable: b.resources.RoutingTable,
	}

	rpMW := &receivePipelineMW{
		comp:      modelComp,
		portIndex: portIndex,
	}

	// Register routeForwardSendMW first (index 0), receivePipelineMW second (index 1).
	// This matches the execution order: sendOut → forward → route → movePipeline → startProcessing
	modelComp.AddMiddleware(rfsMW)
	modelComp.AddMiddleware(rpMW)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}

func (b Builder) routingTableMustBeGiven() {
	if b.resources.RoutingTable == nil {
		panic("switch requires a routing table to operate")
	}
}

// addPort registers a port complex.
func addPort(
	comp *modeling.Component[Spec, State, modeling.None],
	ports *[]messaging.Port,
	portIndex map[messaging.RemotePort]int,
	port messaging.Port,
	remotePort messaging.RemotePort,
	pcs portComplexState,
) {
	idx := len(*ports)
	*ports = append(*ports, port)
	portIndex[remotePort] = idx

	// Also map the local port's RemotePort so route resolution works
	portIndex[port.AsRemote()] = idx

	// Initialize queueing.Buffer fields
	pcs.RouteBuffer = queueing.NewBuffer[routedFlit](
		pcs.LocalPortName+"RouteBuf", pcs.NumInputChannel)
	pcs.ForwardBuffer = queueing.NewBuffer[routedFlit](
		pcs.LocalPortName+"FwdBuf", pcs.NumInputChannel)
	pcs.SendOutBuffer = queueing.NewBuffer[packetization.Flit](
		pcs.LocalPortName+"SendBuf", pcs.NumOutputChannel)
	pcs.Pipeline = queueing.NewPipeline[routedFlit](
		pcs.PipelineWidth, pcs.Latency)

	state := &comp.State
	state.PortComplexes = append(state.PortComplexes, pcs)
}

// SwitchPortAdder can add a port to a switch.
type SwitchPortAdder struct {
	sw               *modeling.Component[Spec, State, modeling.None]
	localPort        messaging.Port
	remotePort       messaging.Port
	latency          int
	numInputChannel  int
	numOutputChannel int
}

// MakeSwitchPortAdder creates a SwitchPortAdder that can add ports for the
// provided switch.
func MakeSwitchPortAdder(sw *modeling.Component[Spec, State, modeling.None]) SwitchPortAdder {
	return SwitchPortAdder{
		sw:               sw,
		numInputChannel:  1,
		numOutputChannel: 1,
		latency:          1,
	}
}

// WithPorts defines the ports to add. The local port is part of the switch.
// The remote port is the port on an endpoint or on another switch.
func (a SwitchPortAdder) WithPorts(local, remote messaging.Port) SwitchPortAdder {
	a.localPort = local
	a.remotePort = remote

	return a
}

// WithLatency sets the latency of the port.
func (a SwitchPortAdder) WithLatency(latency int) SwitchPortAdder {
	a.latency = latency
	return a
}

// WithNumInputChannel sets the number of input channels of the port. This
// number determines the number of flits that can be injected into the switch
// from the port in each cycle.
func (a SwitchPortAdder) WithNumInputChannel(num int) SwitchPortAdder {
	a.numInputChannel = num
	return a
}

// WithNumOutputChannel sets the number of output channels of the port. This
// number determines the number of flits that can be ejected from the switch
// to the port in each cycle.
func (a SwitchPortAdder) WithNumOutputChannel(num int) SwitchPortAdder {
	a.numOutputChannel = num
	return a
}

// AddPort adds the port to the switch.
func (a SwitchPortAdder) AddPort() {
	pcs := portComplexState{
		LocalPortName:    a.localPort.Name(),
		RemotePort:       a.remotePort.AsRemote(),
		NumInputChannel:  a.numInputChannel,
		NumOutputChannel: a.numOutputChannel,
		Latency:          a.latency,
		PipelineWidth:    a.numInputChannel,
	}
	rfsMW := routeForwardSendMiddleware(a.sw)
	addPort(rfsMW.comp, &rfsMW.ports, rfsMW.portIndex,
		a.localPort, a.remotePort.AsRemote(), pcs)

	// Keep receivePipelineMW's ports/portIndex in sync
	rpMW := a.sw.Middlewares()[1].(*receivePipelineMW)
	rpMW.ports = rfsMW.ports
	rpMW.portIndex = rfsMW.portIndex
}
