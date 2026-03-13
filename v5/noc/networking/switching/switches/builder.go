package switches

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
)

// DefaultSpec provides the default configuration for switch components.
var DefaultSpec = Spec{
	Freq: 1 * sim.GHz,
}

// Builder can help building switches
type Builder struct {
	engine       sim.Engine
	spec         Spec
	routingTable routing.Table
}

func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine that the switch to build uses.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the switch to build works at.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithRoutingTable sets the routing table to be used by the switch to build.
func (b Builder) WithRoutingTable(rt routing.Table) Builder {
	b.routingTable = rt
	return b
}

// Build creates a new switch
func (b Builder) Build(name string) *Comp {
	b.engineMustBeGiven()
	b.freqMustNotBeZero()
	b.routingTableMustBeGiven()

	spec := b.spec
	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)

	portIndex := make(map[sim.RemotePort]int)

	rfsMW := &routeForwardSendMW{
		comp:         modelComp,
		portIndex:    portIndex,
		routingTable: b.routingTable,
	}

	rpMW := &receivePipelineMW{
		comp:      modelComp,
		portIndex: portIndex,
	}

	s := &Comp{
		Component: modelComp,
	}

	// Register routeForwardSendMW first (index 0), receivePipelineMW second (index 1).
	// This matches the execution order: sendOut → forward → route → movePipeline → startProcessing
	s.AddMiddleware(rfsMW)
	s.AddMiddleware(rpMW)

	return s
}

func (b Builder) engineMustBeGiven() {
	if b.engine == nil {
		panic("engine of switch is not given")
	}
}

func (b Builder) freqMustNotBeZero() {
	if b.spec.Freq == 0 {
		panic("switch frequency cannot be 0")
	}
}

func (b Builder) routingTableMustBeGiven() {
	if b.routingTable == nil {
		panic("switch requires a routing table to operate")
	}
}

// addPort registers a port complex.
func addPort(
	comp *modeling.Component[Spec, State],
	ports *[]sim.Port,
	portIndex map[sim.RemotePort]int,
	port sim.Port,
	remotePort sim.RemotePort,
	pcs portComplexState,
) {
	idx := len(*ports)
	*ports = append(*ports, port)
	portIndex[remotePort] = idx

	// Also map the local port's RemotePort so route resolution works
	portIndex[port.AsRemote()] = idx

	// Initialize stateutil.Buffer fields
	pcs.RouteBuffer = stateutil.Buffer[routedFlit]{
		BufferName: pcs.LocalPortName + "RouteBuf",
		Cap:        pcs.NumInputChannel,
	}
	pcs.ForwardBuffer = stateutil.Buffer[routedFlit]{
		BufferName: pcs.LocalPortName + "FwdBuf",
		Cap:        pcs.NumInputChannel,
	}
	pcs.SendOutBuffer = stateutil.Buffer[messaging.Flit]{
		BufferName: pcs.LocalPortName + "SendBuf",
		Cap:        pcs.NumOutputChannel,
	}
	pcs.Pipeline = stateutil.Pipeline[routedFlit]{
		Width:     pcs.PipelineWidth,
		NumStages: pcs.Latency,
	}

	// Initialize state in both current and next buffers
	next := comp.GetNextState()
	next.PortComplexes = append(next.PortComplexes, pcs)
	comp.SetState(*next)
}

// SwitchPortAdder can add a port to a switch.
type SwitchPortAdder struct {
	sw               *Comp
	localPort        sim.Port
	remotePort       sim.Port
	latency          int
	numInputChannel  int
	numOutputChannel int
}

// MakeSwitchPortAdder creates a SwitchPortAdder that can add ports for the
// provided switch.
func MakeSwitchPortAdder(sw *Comp) SwitchPortAdder {
	return SwitchPortAdder{
		sw:               sw,
		numInputChannel:  1,
		numOutputChannel: 1,
		latency:          1,
	}
}

// WithPorts defines the ports to add. The local port is part of the switch.
// The remote port is the port on an endpoint or on another switch.
func (a SwitchPortAdder) WithPorts(local, remote sim.Port) SwitchPortAdder {
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
	rfsMW := a.sw.routeForwardSendMiddleware()
	addPort(rfsMW.comp, &rfsMW.ports, rfsMW.portIndex,
		a.localPort, a.remotePort.AsRemote(), pcs)

	// Keep receivePipelineMW's ports/portIndex in sync
	rpMW := a.sw.Middlewares()[1].(*receivePipelineMW)
	rpMW.ports = rfsMW.ports
	rpMW.portIndex = rfsMW.portIndex
}
