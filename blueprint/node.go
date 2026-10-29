// Package blueprint declares simulation assembly without creating runtime objects.
// Expand resolves domains and references; Simulation.Initialize instantiates the
// resulting declaration. Nodes may retain typed construction functions, but never
// runtime objects. JSON code generation is a separate concern.
package blueprint

import (
	"reflect"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Node is an assembly declaration. Use the typed constructors to retain its
// construction information; changing Kind never changes its Go type. The private
// marker seals node kinds; NewNetwork still lets third-party connectors supply
// their own assembly behavior.
type Node interface {
	node()
	Name() string
	setName(string)
}

// nodeName belongs to the declaration snapshot. Add assigns a local name;
// Expand assigns an absolute name without changing the input node.
type nodeName struct{ name string }

func (n nodeName) Name() string { return n.name }

func (n *nodeName) setName(name string) { n.name = name }

// PortConfig explicitly configures both buffers of a component port.
type PortConfig struct {
	Slot string
	Spec twowaybuffered.Spec
}

// ComponentNode declares a component, its local ports, and shared resources.
type ComponentNode struct {
	nodeName
	Kind                               string
	Spec                               any
	Ports                              []PortConfig
	Uses                               map[string]string
	specType, portsType, resourcesType reflect.Type
	create                             func(modeling.Simulation, string, any, reflect.Value) messaging.PortOwner
}

func (*ComponentNode) node() {}

// Comp declares a component using its existing typed model builder. No builder
// is invoked until initialization. All type parameters after C are inferred.
func Comp[C interface{ MakeBuilder(S) B }, S, R any, B interface {
	WithSimulation(modeling.Simulation) B
	WithResources(R) B
	Build(string) C
}](spec S) *ComponentNode {
	t := reflect.TypeFor[C]()
	if t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
		failf("", "Comp", "component must be a pointer to a struct")
	}
	ports, ok := t.Elem().FieldByName("Ports")
	if !ok {
		failf("", "Comp", "component has no Ports")
	}
	return &ComponentNode{Kind: reflect.TypeFor[S]().PkgPath(), Spec: clone(spec), specType: reflect.TypeFor[S](),
		portsType:     ports.Type,
		resourcesType: reflect.TypeFor[R](),
		create: func(s modeling.Simulation, name string, spec any, resources reflect.Value) messaging.PortOwner {
			c := modeling.MakeBuilder[C](spec.(S)).WithSimulation(s).WithResources(resources.Interface().(R)).
				Build(name)
			return any(c).(messaging.PortOwner)
		}}
}

// Port declares one local port. Both capacities must be positive.
func (n *ComponentNode) Port(slot string, spec twowaybuffered.Spec) *ComponentNode {
	n.Ports = append(n.Ports, PortConfig{slot, spec})
	return n
}

// Use binds a Resources field to an absolute resource node path returned by Add.
func (n *ComponentNode) Use(field, path string) *ComponentNode {
	if n.Uses == nil {
		n.Uses = make(map[string]string)
	}
	if _, ok := n.Uses[field]; ok {
		failf("", field, "duplicate resource use")
	}
	n.Uses[field] = path
	return n
}

// ResourceNode declares a named shared resource. Dependencies are constructed
// first, in deterministic dependency order, and injected by Resources field.
type ResourceNode struct {
	nodeName
	Kind                                string
	Spec                                any
	Uses                                map[string]string
	specType, resultType, resourcesType reflect.Type
	create                              func(modeling.Simulation, string, any, reflect.Value) naming.Named
}

func (*ResourceNode) node() {}

// Res declares a resource. Its Spec implements BuildResource with a typed
// Resources struct (modeling.None if empty) and a concrete named result.
// BuildResource constructs and registers exactly one resource when invoked.
func Res[S interface {
	BuildResource(modeling.Simulation, string, R) T
}, R any, T naming.Named](spec S) *ResourceNode {
	return &ResourceNode{Kind: reflect.TypeFor[S]().PkgPath(), Spec: clone(spec), specType: reflect.TypeFor[S](),
		resourcesType: reflect.TypeFor[R](), resultType: reflect.TypeFor[T](),
		create: func(s modeling.Simulation, name string, spec any, r reflect.Value) naming.Named {
			return spec.(S).BuildResource(s, name, r.Interface().(R))
		}}
}

// Use binds a resource dependency by absolute node path.
func (n *ResourceNode) Use(field, path string) *ResourceNode {
	if n.Uses == nil {
		n.Uses = make(map[string]string)
	}
	if _, ok := n.Uses[field]; ok {
		failf("", field, "duplicate resource use")
	}
	n.Uses[field] = path
	return n
}

// DomainNode is a reusable declaration whose build function only adds nodes.
// Its static Ports shape is filled with exposed inner ports after initialization.
type DomainNode struct {
	nodeName
	Kind                string
	Spec                any
	specType, portsType reflect.Type
	expand              func(any, *Blueprint)
}

func (*DomainNode) node() {}

// NewDomain retains a typed declaration function, without executing it.
func NewDomain[S, P any](spec S, build func(S, *Blueprint)) *DomainNode {
	return &DomainNode{Kind: reflect.TypeFor[S]().PkgPath(), Spec: clone(spec), specType: reflect.TypeFor[S](),
		portsType: reflect.TypeFor[P](), expand: func(s any, bp *Blueprint) { build(s.(S), bp) }}
}

// ConnectionNode declares a direct connection. Member order is arbitration order.
type ConnectionNode struct {
	nodeName
	Freq    timing.Freq
	Members []messaging.RemotePort
}

func (*ConnectionNode) node() {}

// NetworkNode declares connector-owned topology. Expand retains this declaration;
// internal switches and endpoints are created only during initialization.
type NetworkNode struct {
	nodeName
	Kind     string
	spec     any
	members  []messaging.RemotePort
	assemble func(any, modeling.Simulation, string, []messaging.Port)
}

func (*NetworkNode) node() {}

// NewNetwork is the extension point for connector packages. Members are resolved
// in order by the framework before assemble runs. The callback creates the
// network using the supplied simulation, name, settings, and real device ports.
// It must not retain or mutate declaration data or create objects before invocation.
func NewNetwork[S any](spec S, members []messaging.RemotePort, assemble func(S, modeling.Simulation, string,
	[]messaging.Port)) *NetworkNode {
	return &NetworkNode{Kind: reflect.TypeFor[S]().PkgPath(), spec: clone(spec),
		members: append([]messaging.RemotePort(nil), members...), assemble: func(spec any,
			s modeling.Simulation, name string, ports []messaging.Port) {
			assemble(spec.(S), s, name, ports)
		}}
}

// Configuration returns an independent copy of the connector settings.
func (n *NetworkNode) Configuration() any { return clone(n.spec) }

// PortReferences returns device references in binding order.
func (n *NetworkNode) PortReferences() []messaging.RemotePort {
	return append([]messaging.RemotePort(nil), n.members...)
}
