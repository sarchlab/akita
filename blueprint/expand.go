package blueprint

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

var portType = reflect.TypeFor[messaging.Port]()
var remoteType = reflect.TypeFor[messaging.RemotePort]()

// Expand produces an independent flat declaration without constructing runtime
// objects. Network declarations remain opaque to expansion. Invalid declarations
// return path-qualified errors; Simulation.Initialize panics with those errors.
func Expand(bp *Blueprint) (out *Blueprint, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
			err = fmt.Errorf("%v", r)
		}
	}()
	if bp == nil {
		failf("", "Expand", "nil blueprint")
	}
	if bp.expanded {
		failf("", "Expand", "already expanded")
	}
	out = &Blueprint{expanded: true, aliases: make(map[string]string)}
	names := make(map[string]bool)
	var expand func([]entry, int)
	expand = func(entries []entry, depth int) {
		if depth > 256 {
			failf("", "domain", "recursive expansion exceeds 256 levels")
		}
		for _, e := range entries {
			if names[e.name] {
				failf(e.name, "name", "duplicate node")
			}
			names[e.name] = true
			n := copyNode(e.node)
			d, ok := n.(*DomainNode)
			if !ok {
				out.entries = append(out.entries, entry{e.name, e.scope, n})
				continue
			}
			if err := modeling.ValidateSpec(d.Spec); err != nil {
				failf(e.name, "Spec", "%v", err)
			}
			child := &Blueprint{scope: e.name}
			contextual(e.name, "domain", func() { d.expand(d.Spec, child) })
			out.domains = append(out.domains, domain{e.name, d.portsType, child.exposes})
			expand(child.entries, depth+1)
		}
	}
	expand(bp.entries, 0)
	out.validate()
	return out, nil
}

func (bp *Blueprint) validate() {
	resources := make(map[string]*ResourceNode)
	names := make(map[string]bool)
	for _, d := range bp.domains {
		names[d.name] = true
	}
	for _, e := range bp.entries {
		names[e.name] = true
		if n, ok := e.node.(*ResourceNode); ok {
			if n.create == nil || n.resultType == nil {
				failf(e.name, "Kind", "use Res to retain construction information")
			}
			resources[e.name] = n
		}
	}
	ports := make(map[string]bool)
	for _, e := range bp.entries {
		validateNode(e, resources, names, ports)
	}
	bp.validateExposures(ports)
	bp.resolveReferences(ports)
	bp.resourceOrder(resources)
}

func validateNode(e entry, resources map[string]*ResourceNode, names, ports map[string]bool) {
	switch n := e.node.(type) {
	case *ComponentNode:
		validateComponent(e.name, n, names, ports)
		validateUses(e.name, n.resourcesType, n.Uses, resources)
	case *ResourceNode:
		validateUses(e.name, n.resourcesType, n.Uses, resources)
	case *ConnectionNode:
		if n.Freq <= 0 {
			failf(e.name, "Freq", "frequency must be positive")
		}
	case *NetworkNode:
		if n.assemble == nil {
			failf(e.name, "Kind", "use NewNetwork to retain construction information")
		}
		if v, ok := n.spec.(interface{ Validate() error }); ok {
			if err := v.Validate(); err != nil {
				failf(e.name, "Spec", "%v", err)
			}
		}
	}
}

func validateComponent(name string, n *ComponentNode, names, ports map[string]bool) {
	if n.create == nil {
		failf(name, "Kind", "use Comp to retain construction information")
	}
	if err := modeling.ValidateSpec(n.Spec); err != nil {
		failf(name, "Spec", "%v", err)
	}
	configured := make(map[string]string)
	for _, p := range n.Ports {
		if _, ok := configured[p.Slot]; ok {
			failf(name, p.Slot, "duplicate port configuration")
		}
		if p.Spec.IncomingCapacity <= 0 || p.Spec.OutgoingCapacity <= 0 {
			failf(name, p.Slot, "both buffer capacities must be positive")
		}
		configured[p.Slot] = join(name, p.Slot)
	}
	for _, slot := range portSlots(name, n.portsType, configured) {
		path := join(name, slot)
		if names[path] || ports[path] {
			failf(name, slot, "duplicate name %s", path)
		}
		ports[path] = true
	}
}

func (bp *Blueprint) validateExposures(ports map[string]bool) {
	for _, d := range bp.domains {
		exposed := make(map[string]string)
		for _, x := range d.exposes {
			if _, ok := exposed[x.field]; ok {
				failf(d.name, x.field, "duplicate expose")
			}
			exposed[x.field] = x.target
		}
		for _, slot := range portSlots(d.name, d.portsType, exposed) {
			alias := join(d.name, slot)
			if ports[alias] {
				failf(d.name, slot, "exposed name collides with an inner port")
			}
			if _, ok := bp.aliases[alias]; ok {
				failf(d.name, slot, "duplicate exposed name")
			}
			bp.aliases[alias] = exposed[slot]
		}
	}
	for _, d := range bp.domains {
		for _, x := range d.exposes {
			bp.checkBoundary(d.name, x.target, d.name, x.field)
			bp.resolve(x.target, ports, make(map[string]bool), d.name, x.field)
		}
	}
}

func (bp *Blueprint) resolveReferences(ports map[string]bool) {
	bound := make(map[string]string)
	for _, e := range bp.entries {
		resolve := func(ref messaging.RemotePort, field string) messaging.RemotePort {
			if ref == messaging.Unconnected {
				return ref
			}
			bp.checkBoundary(e.scope, string(ref), e.name, field)
			return messaging.RemotePort(bp.resolve(string(ref), ports, make(map[string]bool), e.name, field))
		}
		switch n := e.node.(type) {
		case *ComponentNode:
			n.Spec = resolveSpec(n.Spec, resolve)
		case *ResourceNode:
			n.Spec = resolveSpec(n.Spec, resolve)
		case *ConnectionNode:
			n.Members = resolveMembers(n.Members, resolve, e.name, bound)
		case *NetworkNode:
			n.spec = resolveSpec(n.spec, resolve)
			n.members = resolveMembers(n.members, resolve, e.name, bound)
		}
	}
}

func contextual(path, field string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			failf(path, field, "%v", r)
		}
	}()
	fn()
}

func portSlots(path string, t reflect.Type, configured map[string]string) []string {
	if t == nil || t.Kind() != reflect.Struct {
		failf(path, "Ports", "must be a struct")
	}
	known := make(map[string]bool)
	var slots []string
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			failf(path, "Ports."+f.Name, "must be exported")
		}
		if f.Type == portType {
			if _, ok := configured[f.Name]; !ok {
				failf(path, "Ports."+f.Name, "missing configuration or expose")
			}
			slots = append(slots, f.Name)
			known[f.Name] = true
			continue
		}
		if f.Type != reflect.SliceOf(portType) {
			failf(path, "Ports."+f.Name, "must be messaging.Port or []messaging.Port")
		}
		indices := make(map[int]string)
		for _, slot := range keys(configured) {
			if !strings.HasPrefix(slot, f.Name+"[") {
				continue
			}
			suffix := strings.TrimPrefix(slot, f.Name+"[")
			num := strings.TrimSuffix(suffix, "]")
			index, err := strconv.Atoi(num)
			if err != nil || index < 0 || slot != fmt.Sprintf("%s[%d]", f.Name, index) {
				failf(path, slot, "invalid group index")
			}
			indices[index] = slot
		}
		for j := 0; j < len(indices); j++ {
			slot, ok := indices[j]
			if !ok {
				failf(path, f.Name, "group indices must be contiguous from zero")
			}
			slots = append(slots, slot)
			known[slot] = true
		}
	}
	for _, slot := range keys(configured) {
		if !known[slot] {
			failf(path, slot, "unknown port slot")
		}
	}
	return slots
}
func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func validateUses(path string, t reflect.Type, uses map[string]string, resources map[string]*ResourceNode) {
	if t == nil || t.Kind() != reflect.Struct {
		failf(path, "Resources", "must be a struct")
	}
	fields := make(map[string]bool)
	for i := range t.NumField() {
		f := t.Field(i)
		fields[f.Name] = true
		if !f.IsExported() {
			failf(path, "Uses."+f.Name, "resource field must be exported")
		}
		target, ok := uses[f.Name]
		if !ok {
			failf(path, "Uses."+f.Name, "missing resource use")
		}
		r := resources[target]
		if r == nil {
			failf(path, "Uses."+f.Name, "unknown resource %q", target)
		}
		if !r.resultType.AssignableTo(f.Type) {
			failf(path, "Uses."+f.Name, "resource %q has type %s, need %s", target, r.resultType, f.Type)
		}
	}
	for _, field := range keys(uses) {
		if !fields[field] {
			failf(path, "Uses."+field, "unknown resource field")
		}
	}
}
func (bp *Blueprint) checkBoundary(scope, target, path, field string) {
	// Entering a foreign domain is only allowed through its public alias.
	for _, d := range bp.domains {
		if d.name == "" || within(scope, d.name) || !within(target, d.name) {
			continue
		}
		suffix := strings.TrimPrefix(target, d.name+".")
		if _, ok := bp.aliases[join(d.name, suffix)]; !ok {
			failf(path, field, "reference %q crosses unexposed domain %q", target, d.name)
		}
		// An alias belongs to this domain, not merely to a descendant domain.
		found := false
		for _, x := range d.exposes {
			if suffix == x.field {
				found = true
				break
			}
		}
		if !found {
			failf(path, field, "reference %q is not exposed by domain %q", target, d.name)
		}
	}
}
func (bp *Blueprint) resolve(ref string, ports, seen map[string]bool, path, field string) string {
	if seen[ref] {
		failf(path, field, "expose alias cycle at %q", ref)
	}
	seen[ref] = true
	if next, ok := bp.aliases[ref]; ok {
		return bp.resolve(next, ports, seen, path, field)
	}
	if !ports[ref] {
		failf(path, field, "unknown port reference %q", ref)
	}
	return ref
}
func resolveSpec(spec any, resolve func(messaging.RemotePort, string) messaging.RemotePort) any {
	value := cloneValue(reflect.ValueOf(spec))
	var visit func(reflect.Value, string)
	visit = func(v reflect.Value, path string) {
		if v.Type() == remoteType {
			if v.String() == "" {
				return
			} // An unset optional field is not a reference; model validation owns requiredness.
			v.SetString(string(resolve(messaging.RemotePort(v.String()), path)))
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := range v.NumField() {
				visit(v.Field(i), path+"."+v.Type().Field(i).Name)
			}
		case reflect.Array, reflect.Slice:
			for i := range v.Len() {
				visit(v.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	visit(value, "Spec")
	return value.Interface()
}
func resolveMembers(in []messaging.RemotePort, resolve func(messaging.RemotePort,
	string) messaging.RemotePort, path string, bound map[string]string) []messaging.RemotePort {
	out := make([]messaging.RemotePort, len(in))
	for i, r := range in {
		field := fmt.Sprintf("Members[%d]", i)
		if r == messaging.Unconnected {
			failf(path, field, "Unconnected cannot be attached")
		}
		out[i] = resolve(r, field)
		key := string(out[i])
		if prior, ok := bound[key]; ok {
			failf(path, field, "port %q already attached to %s", key, prior)
		}
		bound[key] = path
	}
	return out
}
func (bp *Blueprint) resourceOrder(resources map[string]*ResourceNode) []string {
	state := make(map[string]int)
	var order []string
	var visit func(string)
	visit = func(name string) {
		if state[name] == 2 {
			return
		}
		if state[name] == 1 {
			failf(name, "Uses", "resource dependency cycle")
		}
		state[name] = 1
		n := resources[name]
		for _, field := range keys(n.Uses) {
			visit(n.Uses[field])
		}
		state[name] = 2
		order = append(order, name)
	}
	for _, e := range bp.entries {
		if _, ok := resources[e.name]; ok {
			visit(e.name)
		}
	}
	return order
}
