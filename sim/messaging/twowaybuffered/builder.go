package twowaybuffered

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/queueing"
)

// Spec configures the capacities of a port's independent buffers.
type Spec struct {
	IncomingCapacity int `json:"incoming_capacity"`
	OutgoingCapacity int `json:"outgoing_capacity"`
}

// DefaultSpec returns a one-entry buffer in each direction.
func DefaultSpec() Spec { return Spec{IncomingCapacity: 1, OutgoingCapacity: 1} }

// Builder constructs unnamed ports. Component.BindPort assigns a port's owner,
// name and registration; Connection.BindPort attaches the connection.
type Builder struct {
	simulation modeling.Simulation
	spec       Spec
}

// MakeBuilder returns a builder configured with DefaultSpec.
func MakeBuilder() Builder { return Builder{spec: DefaultSpec()} }

// WithSimulation supplies the simulation the port belongs to. It is required.
func (b Builder) WithSimulation(s modeling.Simulation) Builder { b.simulation = s; return b }

// WithSpec supplies the buffer capacities. Both must be positive.
func (b Builder) WithSpec(spec Spec) Builder { b.spec = spec; return b }

// Build creates a fresh port without assigning a name, owner or connection.
func (b Builder) Build() *Port {
	if b.simulation == nil ||
		(reflect.ValueOf(b.simulation).Kind() == reflect.Pointer && reflect.ValueOf(b.simulation).IsNil()) {
		panic("twowaybuffered: WithSimulation is required")
	}
	assembly.RequireSetup(b.simulation)
	if b.spec.IncomingCapacity <= 0 {
		panic(fmt.Sprintf("twowaybuffered: incoming capacity must be positive, got %d", b.spec.IncomingCapacity))
	}
	if b.spec.OutgoingCapacity <= 0 {
		panic(fmt.Sprintf("twowaybuffered: outgoing capacity must be positive, got %d", b.spec.OutgoingCapacity))
	}
	return &Port{simulation: b.simulation, portAssembly: assembly.NewMember(b.simulation, nil, nil),
		incomingBuf: queueing.MakeBuffer[bufferEntry](b.spec.IncomingCapacity),
		outgoingBuf: queueing.MakeBuffer[bufferEntry](b.spec.OutgoingCapacity)}
}
