// Package schema defines the versioned JSON output of the akita source
// inspector. External tools consume this schema instead of hand-written
// manifest files; it is extracted statically from Go source, so it always
// matches the declarations the runtime executes.
package schema

// Version is the current schema version, carried by every emitted
// definition as "schema_version". Additive changes (new optional fields) do
// not bump it; renames, removals, or semantic changes do.
const Version = 1

// Definition kinds. The envelope is shared across kinds; kind-specific
// fields are simply absent for kinds that do not use them.
const (
	KindComponent = "component"
)

// Component models: how a component runs. Definitions declared with the
// legacy modeling.ComponentDef carry no model.
const (
	// ModelTicking is a component that runs on a clock (modeling/ticking).
	ModelTicking = "ticking"

	// ModelWakeup is a component without a clock that runs when woken
	// (modeling/wakeup).
	ModelWakeup = "wakeup"

	// ModelEvent is a component without a clock whose behavior is a set of
	// reactions to events (modeling/event).
	ModelEvent = "event"
)

// Definition describes one definition found in a package.
type Definition struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`

	// Model says how a component runs, e.g. "ticking".
	Model string `json:"model,omitempty"`

	// Name is the definition's display name. A component declared with a
	// component model is identified by its package, so its Name is the
	// package name.
	Name    string `json:"name"`
	Package string `json:"package"`
	Module  string `json:"module,omitempty"`

	// Spec lists the configurable fields of the definition's Spec (or
	// parameter) type, in declaration order.
	Spec []Field `json:"spec"`

	// Resources lists the fields of the component's Resources type: the
	// external references that must be supplied at construction time.
	Resources []Field `json:"resources,omitempty"`

	// Ports describes the component's boundary ports and port groups
	// (component kind only).
	Ports []Port `json:"ports,omitempty"`

	// Middlewares lists the component's middlewares in the order it runs them
	// on every tick (component kind only, for definitions that declare them).
	Middlewares []Middleware `json:"middlewares,omitempty"`
}

// Middleware describes one field of a component's Middlewares struct.
type Middleware struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"doc,omitempty"`
}

// Field describes one field of a Spec or Resources struct.
type Field struct {
	Name     string `json:"name"`
	JSONName string `json:"json_name,omitempty"`
	Type     string `json:"type"`
	Doc      string `json:"doc,omitempty"`

	// Default is the value the definition's DefaultSpec assigns to the
	// field; fields the literal leaves out get their Go zero value. Spec
	// fields are scalars, so defaults are too. Absent for Resources fields.
	// Values describe Go fields before JSON marshaling, so json:",string"
	// does not transform defaults.
	Default any `json:"default,omitempty"`

	// Unit is the semantic unit implied by the field's Go type (e.g. "Hz"
	// for timing.Freq).
	Unit string `json:"unit,omitempty"`

	// Derived marks fields computed from other fields at build time; they
	// are not user-configurable.
	Derived bool `json:"derived,omitempty"`

	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`

	// Choices lists the declared constants of the field's named string
	// type, when it has any.
	Choices []string `json:"choices,omitempty"`
}

// Role identifies one protocol role a port speaks.
type Role struct {
	Protocol string `json:"protocol"`
	Role     string `json:"role"`
}

// Port describes one declared port or port group.
type Port struct {
	Name  string `json:"name"`
	Roles []Role `json:"roles,omitempty"`

	// Group marks a dynamically-sized port group. Members are addressed
	// "Name[0]" ... "Name[N-1]", dense and zero-indexed; N is decided at
	// configuration time. All members speak Roles.
	Group bool `json:"group,omitempty"`
}
