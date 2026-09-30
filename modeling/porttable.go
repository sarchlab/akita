package modeling

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
)

var (
	portType      = reflect.TypeFor[messaging.Port]()
	portSliceType = reflect.TypeFor[[]messaging.Port]()
)

// PortTable gives name-based access to a component's Ports struct: a struct
// whose fields are the component's ports, one messaging.Port field per port
// and one []messaging.Port field per port group. The component kinds (see
// modeling/ticking) keep a typed Ports field and use a PortTable to implement
// the name-based side of messaging.PortOwner.
type PortTable struct {
	owner  messaging.Component
	ports  reflect.Value
	layout *portLayout
}

// NewPortTable binds every port in the Ports struct that ports points to. Each
// fixed port must be set and named "<owner>.<field>"; a port group may be
// empty and grow with AssignPortToGroup. The set of ports is fixed by the
// struct type, so no port can be added later.
func NewPortTable(owner messaging.Component, ports any) *PortTable {
	v := reflect.ValueOf(ports)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf(
			"modeling: NewPortTable needs a pointer to a Ports struct, got %T",
			ports))
	}

	t := &PortTable{
		owner:  owner,
		ports:  v.Elem(),
		layout: portLayoutOf(v.Elem().Type()),
	}
	t.bindAll()

	return t
}

// GetPortByName returns the port stored in the field with the given name, or
// member i of a port group addressed as "Group[i]".
func (t *PortTable) GetPortByName(name string) messaging.Port {
	if f, ok := t.layout.byName[name]; ok && !f.group {
		return t.ports.Field(f.index).Interface().(messaging.Port)
	}

	if group, idx, ok := parseGroupMember(name); ok {
		if f, found := t.layout.byName[group]; found && f.group {
			members := t.ports.Field(f.index)
			if idx < members.Len() {
				return members.Index(idx).Interface().(messaging.Port)
			}
		}
	}

	panic(fmt.Sprintf("modeling: component %q has no port %q; ports are %s",
		t.owner.Name(), name, strings.Join(t.layout.names(), ", ")))
}

// AssignPortToGroup binds a port to the owner and appends it to the named port
// group. It returns the member's name, "Group[i]".
func (t *PortTable) AssignPortToGroup(group string, port messaging.Port) string {
	members := t.group(group)
	name := group + "[" + strconv.Itoa(members.Len()) + "]"

	t.bind(port, "")
	members.Set(reflect.Append(members, reflect.ValueOf(port)))

	return name
}

// PortsInGroup returns the members of the named port group, in the order they
// were added.
func (t *PortTable) PortsInGroup(group string) []messaging.Port {
	members := t.group(group)

	out := make([]messaging.Port, members.Len())
	for i := range out {
		out[i] = members.Index(i).Interface().(messaging.Port)
	}

	return out
}

// NumPortsInGroup returns the number of members of the named port group.
func (t *PortTable) NumPortsInGroup(group string) int {
	return t.group(group).Len()
}

// AllPorts returns every port: the fixed ports in field order, then the
// members of each port group.
func (t *PortTable) AllPorts() []messaging.Port {
	var out []messaging.Port

	for _, f := range t.layout.fields {
		v := t.ports.Field(f.index)
		if !f.group {
			if !v.IsNil() {
				out = append(out, v.Interface().(messaging.Port))
			}
			continue
		}

		for i := 0; i < v.Len(); i++ {
			out = append(out, v.Index(i).Interface().(messaging.Port))
		}
	}

	return out
}

// bindAll binds every port given at Build.
func (t *PortTable) bindAll() {
	for _, f := range t.layout.fields {
		v := t.ports.Field(f.index)

		if f.group {
			for i := 0; i < v.Len(); i++ {
				t.bind(v.Index(i).Interface().(messaging.Port), "")
			}
			continue
		}

		if v.IsNil() {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is not given; pass it with WithPorts",
				t.owner.Name(), f.name))
		}

		t.bind(v.Interface().(messaging.Port), f.name)
	}
}

// bind makes the owner the port's component. A non-empty field is the Ports
// field the port fills, whose full name the port must carry.
func (t *PortTable) bind(port messaging.Port, field string) {
	name := t.owner.Name()

	if field != "" {
		if want := name + "." + field; port.Name() != want {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is named %q, want %q",
				name, field, port.Name(), want))
		}
	}

	if owner := port.Component(); owner != nil && owner != t.owner {
		panic(fmt.Sprintf(
			"modeling: component %q: port %q already belongs to %q",
			name, port.Name(), owner.Name()))
	}

	port.SetComponent(t.owner)
}

// group returns the settable slice of the named port group.
func (t *PortTable) group(group string) reflect.Value {
	f, ok := t.layout.byName[group]
	if !ok || !f.group {
		panic(fmt.Sprintf("modeling: component %q has no port group %q",
			t.owner.Name(), group))
	}

	return t.ports.Field(f.index)
}

// portField describes one field of a Ports struct.
type portField struct {
	name  string
	index int
	group bool
}

// portLayout maps the fields of a Ports struct to port names. It is computed
// once per type.
type portLayout struct {
	fields []portField
	byName map[string]portField
}

var portLayouts sync.Map // reflect.Type -> *portLayout

// portLayoutOf returns the layout of a Ports struct type. It panics unless
// every field is an exported messaging.Port or []messaging.Port.
func portLayoutOf(t reflect.Type) *portLayout {
	if l, ok := portLayouts.Load(t); ok {
		return l.(*portLayout)
	}

	l := &portLayout{byName: map[string]portField{}}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			panic(fmt.Sprintf(
				"modeling: Ports field %s.%s must be exported", t, f.Name))
		}

		var group bool
		switch f.Type {
		case portType:
		case portSliceType:
			group = true
		default:
			panic(fmt.Sprintf(
				"modeling: Ports field %s.%s must be messaging.Port or "+
					"[]messaging.Port, not %s", t, f.Name, f.Type))
		}

		pf := portField{name: f.Name, index: i, group: group}
		l.fields = append(l.fields, pf)
		l.byName[f.Name] = pf
	}

	actual, _ := portLayouts.LoadOrStore(t, l)

	return actual.(*portLayout)
}

// names lists the port names, with groups shown as "Group[]".
func (l *portLayout) names() []string {
	out := make([]string, len(l.fields))
	for i, f := range l.fields {
		out[i] = f.name
		if f.group {
			out[i] += "[]"
		}
	}

	return out
}

// parseGroupMember splits "Group[i]" into the group name and index.
func parseGroupMember(name string) (string, int, bool) {
	open := strings.IndexByte(name, '[')
	if open <= 0 || !strings.HasSuffix(name, "]") {
		return "", 0, false
	}

	idx, err := strconv.Atoi(name[open+1 : len(name)-1])
	if err != nil || idx < 0 {
		return "", 0, false
	}

	return name[:open], idx, true
}
