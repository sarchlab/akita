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

// Definition describes one definition found in a package.
type Definition struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Package       string `json:"package"`
	Module        string `json:"module,omitempty"`

	// Spec lists the configurable fields of the definition's Spec (or
	// parameter) type, in declaration order.
	Spec []Field `json:"spec"`

	// Resources lists the fields of the component's Resources type: the
	// external references that must be supplied at construction time.
	Resources []Field `json:"resources,omitempty"`

	// Ports and PortGroups describe the component's boundary ports
	// (component kind only).
	Ports      []Port      `json:"ports,omitempty"`
	PortGroups []PortGroup `json:"port_groups,omitempty"`
}

// Field describes one field of a Spec or Resources struct.
type Field struct {
	Name     string `json:"name"`
	JSONName string `json:"json_name,omitempty"`
	Type     string `json:"type"`
	Doc      string `json:"doc,omitempty"`

	// Default is the value the definition's DefaultSpec assigns to the
	// field; fields the literal leaves out get their Go zero value. Absent
	// for Resources fields and for nil slices/maps.
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

// Port describes one declared port.
type Port struct {
	Name  string `json:"name"`
	Roles []Role `json:"roles,omitempty"`
}

// PortGroup describes a dynamically-sized group of ports. Members are
// addressed "Name[0]" ... "Name[N-1]", dense and zero-indexed; N is decided
// at configuration time within [MinCount, MaxCount].
type PortGroup struct {
	Name  string `json:"name"`
	Roles []Role `json:"roles,omitempty"`

	MinCount int `json:"min_count,omitempty"`
	// MaxCount of 0 means unbounded.
	MaxCount int `json:"max_count,omitempty"`

	// CountField names the Spec field (by JSON name) that holds the
	// configured group size, if the component binds one.
	CountField string `json:"count_field,omitempty"`
}
