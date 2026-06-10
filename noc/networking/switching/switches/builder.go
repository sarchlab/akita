package switches

import (
	"fmt"

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

	// The switch has a dynamic number of ports, added later with
	// MakeSwitchPortAdder. They live in the "Port" group.
	modelComp.DeclarePortGroup("Port", packetization.Link)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}

func (b Builder) routingTableMustBeGiven() {
	if b.resources.RoutingTable == nil {
		panic("switch requires a routing table to operate")
	}
}

// addPort registers a port complex for an already-created, group-assigned local
// port. The local port lives in the switch's "Port" group; State.PortComplexes
// is kept index-aligned with that group.
func addPort(
	comp *modeling.Component[Spec, State, modeling.None],
	portIndex map[messaging.RemotePort]int,
	port messaging.Port,
	remotePort messaging.RemotePort,
	pcs portComplexState,
) {
	idx := len(comp.State.PortComplexes)

	// The remote peer may be unknown at this point (switch-to-switch links wire
	// the second side later with SetPortRemote).
	if remotePort != "" {
		portIndex[remotePort] = idx
	}

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

// SwitchPortAdder mints a port on a switch connected to a remote peer (another
// switch's port or an endpoint's NetworkPort). The local port is created,
// registered with the registrar, and appended to the switch's "Port" group; the
// port complex (channels, latency, buffers, routing) it sets up is internal to
// the switch. Externally you only supply the remote peer.
type SwitchPortAdder struct {
	sw               *modeling.Component[Spec, State, modeling.None]
	registrar        modeling.Registrar
	remotePort       messaging.Port
	bufSize          int
	latency          int
	numInputChannel  int
	numOutputChannel int
}

// MakeSwitchPortAdder creates a SwitchPortAdder that can add ports for the
// provided switch.
func MakeSwitchPortAdder(sw *modeling.Component[Spec, State, modeling.None]) SwitchPortAdder {
	return SwitchPortAdder{
		sw:               sw,
		bufSize:          1,
		numInputChannel:  1,
		numOutputChannel: 1,
		latency:          1,
	}
}

// WithRegistrar sets the registrar used to register the minted local port.
func (a SwitchPortAdder) WithRegistrar(reg modeling.Registrar) SwitchPortAdder {
	a.registrar = reg
	return a
}

// WithRemotePort sets the peer this port connects to — another switch's port or
// an endpoint's NetworkPort.
func (a SwitchPortAdder) WithRemotePort(remote messaging.Port) SwitchPortAdder {
	a.remotePort = remote
	return a
}

// WithBufferSize sets the buffer size of the minted local port (default 1).
func (a SwitchPortAdder) WithBufferSize(size int) SwitchPortAdder {
	a.bufSize = size
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

// Add mints the local port, registers it, appends it to the switch's "Port"
// group, builds the internal port complex toward the remote peer, and returns
// the new local port.
func (a SwitchPortAdder) Add() messaging.Port {
	if a.registrar == nil {
		panic("switches: SwitchPortAdder requires a registrar")
	}

	idx := a.sw.NumPortsInGroup("Port")
	local := modeling.MakePortBuilder().
		WithRegistrar(a.registrar).
		WithComponent(a.sw).
		WithSpec(modeling.PortSpec{BufSize: a.bufSize}).
		Build(fmt.Sprintf("Port[%d]", idx))
	a.sw.AssignPortToGroup("Port", local)

	// The remote peer is optional: switch-to-switch links wire the second side
	// later with SetPortRemote, since both local ports must exist first.
	var remoteName messaging.RemotePort
	if a.remotePort != nil {
		remoteName = a.remotePort.AsRemote()
	}

	pcs := portComplexState{
		LocalPortName:    local.Name(),
		RemotePort:       remoteName,
		NumInputChannel:  a.numInputChannel,
		NumOutputChannel: a.numOutputChannel,
		Latency:          a.latency,
		PipelineWidth:    a.numInputChannel,
	}

	// portIndex is shared between the two middlewares (same map), and the local
	// ports live in the component's "Port" group, so nothing needs syncing.
	rfsMW := routeForwardSendMiddleware(a.sw)
	addPort(a.sw, rfsMW.portIndex, local, remoteName, pcs)

	return local
}

// SetPortRemote records the remote peer for a port already added with Add. It is
// used for switch-to-switch links, where both local ports must exist before
// either side's route to the other can be resolved.
func SetPortRemote(
	sw *modeling.Component[Spec, State, modeling.None],
	local, remote messaging.Port,
) {
	rfsMW := routeForwardSendMiddleware(sw)
	state := &sw.State

	for i := range state.PortComplexes {
		if state.PortComplexes[i].LocalPortName == local.Name() {
			state.PortComplexes[i].RemotePort = remote.AsRemote()
			rfsMW.portIndex[remote.AsRemote()] = i
			return
		}
	}

	panic(fmt.Sprintf("%s: local port %s not found in port complexes",
		sw.Name(), local.Name()))
}
