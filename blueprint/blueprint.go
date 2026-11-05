package blueprint

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type entry struct {
	scope string
	node  Node
}

type exposure struct{ field, target string }

type domain struct {
	name      string
	portsType reflect.Type
	exposes   []exposure
}

// Blueprint is an ordered assembly declaration. Its zero value is ready to use.
// A domain receives a path-scoped Blueprint; do not retain it after expansion.
type Blueprint struct {
	scope    string
	entries  []entry
	exposes  []exposure
	expanded bool
	exports  []portExport
}

// Add snapshots a node and returns its absolute path. Only a root domain may
// have an empty name. Other names must be a single path segment. The same input
// node may be reused in independent domains.
// The returned path is already absolute; do not pass it to PortRef.
func (bp *Blueprint) Add(name string, n Node) string {
	if bp.expanded {
		failf(bp.scope, "Add", "expanded blueprint is immutable")
	}
	path := join(bp.scope, name)
	if name == "" {
		if bp.scope != "" {
			failf(path, "name", "only root may have an empty name")
		}
		if _, ok := n.(*DomainNode); !ok {
			failf(path, "name", "empty name requires a root domain")
		}
	} else {
		if strings.Contains(name, ".") {
			failf(path, "name", "node name must be a single path segment")
		}
		checkName(path, "name", name)
	}
	if n == nil || reflect.ValueOf(n).IsNil() {
		failf(path, "node", "nil node")
	}
	snapshot := copyNode(n)
	snapshot.setName(name)
	bp.entries = append(bp.entries, entry{bp.scope, snapshot})
	return path
}

// PortRef creates an absolute port address from a path relative to the current
// domain declaration environment, not the simulation root or filesystem.
// It always prefixes this domain's path, even when path starts with that name.
// Pass already absolute references through Specs unchanged, without calling
// PortRef again. Exposed aliases are resolved during Expand.
func (bp *Blueprint) PortRef(path string) messaging.RemotePort {
	checkName(bp.scope, "PortRef", path)
	return messaging.RemotePort(join(bp.scope, path))
}

// Connect declares a direct connection, preserving the supplied member order.
func (bp *Blueprint) Connect(name string, freq timing.Freq, members ...messaging.RemotePort) {
	bp.Add(name, &ConnectionNode{Freq: freq, Members: members})
}

// Expose binds a domain's public Ports slot to an inner relative port path.
func (bp *Blueprint) Expose(field, inner string) {
	bp.exposes = append(bp.exposes, exposure{field, join(bp.scope, inner)})
}

// Errorf aborts expansion with a path-qualified diagnostic.
func (bp *Blueprint) Errorf(format string, args ...any) { failf(bp.scope, "domain", format, args...) }

// Empty reports whether this declaration contains no nodes.
func (bp *Blueprint) Empty() bool { return len(bp.entries) == 0 }

// Nodes returns a detached copy of the expanded (or declared) nodes in order.
// Domains are absent from an expanded blueprint; networks remain declarations.
func (bp *Blueprint) Nodes() []Node {
	out := make([]Node, len(bp.entries))
	for i, e := range bp.entries {
		out[i] = copyNode(e.node)
	}
	return out
}

// portExport is a flat named Ports value with fully resolved field bindings.
type portExport struct {
	name      string
	portsType reflect.Type
	bindings  []exposure
}

func join(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "." + name
}

func within(path, scope string) bool {
	return scope == "" || path == scope || strings.HasPrefix(path, scope+".")
}

func failf(path, field, format string, args ...any) {
	if path == "" {
		path = "<root>"
	}
	panic(fmt.Errorf("blueprint: %s.%s: %s", path, field, fmt.Sprintf(format, args...)))
}

func checkName(path, field, name string) {
	defer func() {
		if r := recover(); r != nil {
			failf(path, field, "%v", r)
		}
	}()
	naming.MustBeValid(name)
}

func clone[T any](v T) T {
	if !reflect.ValueOf(v).IsValid() {
		return v
	}
	return cloneValue(reflect.ValueOf(v)).Interface().(T)
}

func cloneValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	switch v.Kind() {
	case reflect.Interface:
		out := reflect.New(v.Type()).Elem()
		if !v.IsNil() {
			out.Set(cloneValue(v.Elem()))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := range v.NumField() {
			if !out.Field(i).CanSet() || !v.Type().Field(i).IsExported() {
				failf("", "Spec", "unexported field %s", v.Type().Field(i).Name)
			}
			out.Field(i).Set(cloneValue(v.Field(i)))
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(cloneValue(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(cloneValue(v.Index(i)))
		}
		return out
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		failf("", "Spec", "runtime objects and %s values are not declaration data", v.Kind())
	}
	return v
}

func copyUses(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyNode(n Node) Node {
	switch n := n.(type) {
	case *ComponentNode:
		o := *n
		o.Spec = clone(n.Spec)
		o.Ports = append([]PortConfig(nil), n.Ports...)
		o.Uses = copyUses(n.Uses)
		return &o
	case *ResourceNode:
		o := *n
		o.Spec = clone(n.Spec)
		o.Uses = copyUses(n.Uses)
		return &o
	case *DomainNode:
		o := *n
		o.Spec = clone(n.Spec)
		return &o
	case *ConnectionNode:
		o := *n
		o.Members = append([]messaging.RemotePort(nil), n.Members...)
		return &o
	case *NetworkNode:
		o := *n
		o.spec = clone(n.spec)
		o.members = append([]messaging.RemotePort(nil), n.members...)
		return &o
	default:
		panic("unreachable node type")
	}
}
