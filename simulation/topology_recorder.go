package simulation

import (
	"encoding/json"
	"reflect"

	"github.com/sarchlab/akita/v5/datarecording"
)

// componentSpecTableName holds one row per component: its name, the Go type of
// its spec, and the spec serialized as JSON.
const componentSpecTableName = "component_spec"

// portTableName holds one row per registered port: the component that owns it
// and the connection it is plugged into (empty when the port is unconnected).
// It is the complete port inventory, so the index page can show every port of a
// component, including unconnected control/debug ports. The connection graph is
// the subset with a non-empty Connection: ports sharing a Connection are that
// connection's endpoints (a connection may join more than two ports).
const portTableName = "port"

// componentSpecEntry is one row of the component_spec table. Spec is the
// JSON-encoded spec, or empty when the component exposes no spec or the spec
// does not serialize.
type componentSpecEntry struct {
	Name string
	Type string
	Spec string
}

// portEntry is one row of the port table. Connection is empty for an
// unconnected port.
type portEntry struct {
	Component  string
	Port       string
	Connection string
}

// named is anything that can report its name. Specs, components, ports, and
// connections all satisfy it; the recorder relies on it to read names out of
// reflectively obtained values without the simulation package depending on the
// messaging package.
type named interface {
	Name() string
}

// topologyRecorder records the static structure of a simulation — every
// component's spec and the full port inventory with its connection graph — into
// the recording, making it self-describing for tools such as Daisen's index
// page.
//
// Specs are read through the public Spec accessor that every modeling.Component
// exposes; because that accessor is generic there is no single non-generic
// interface to assert against, so the recorder reaches it by reflection. This
// runs once at Terminate, so its cost is irrelevant to simulation speed.
type topologyRecorder struct {
	recorder datarecording.DataRecorder
}

// newTopologyRecorder creates the topology tables up front so they exist even
// for a run that registers nothing. The rows are written later by Record.
func newTopologyRecorder(recorder datarecording.DataRecorder) *topologyRecorder {
	r := &topologyRecorder{recorder: recorder}
	r.recorder.CreateTable(componentSpecTableName, componentSpecEntry{})
	r.recorder.CreateTable(portTableName, portEntry{})

	return r
}

// Record writes the component specs and the port inventory. It is called from
// Terminate, by which point every component, port, and connection is registered.
func (r *topologyRecorder) Record(components []Component, ports []Port) {
	r.recordComponentSpecs(components)
	r.recordPorts(ports)
	r.recorder.Flush()
}

func (r *topologyRecorder) recordComponentSpecs(components []Component) {
	for _, c := range components {
		entry := componentSpecEntry{Name: c.Name()}

		if spec, ok := reflectSpec(c); ok {
			entry.Type = reflect.TypeOf(spec).String()
			if data, err := json.Marshal(spec); err == nil {
				entry.Spec = string(data)
			}
		}

		r.recorder.InsertData(componentSpecTableName, entry)
	}
}

// recordPorts records every registered port together with the connection it is
// plugged into, if any. Unconnected ports are recorded with an empty Connection
// so the table is the complete port inventory, not only the connection graph.
func (r *topologyRecorder) recordPorts(ports []Port) {
	for _, p := range ports {
		// reflectName reports ok=false for an unconnected port (nil
		// Connection) or a port without the accessor; the empty Connection
		// that leaves behind is intentional.
		conn, _ := reflectName(p, "Connection")
		comp, _ := reflectName(p, "Component")

		r.recorder.InsertData(portTableName, portEntry{
			Component:  comp,
			Port:       p.Name(),
			Connection: conn,
		})
	}
}

// reflectSpec returns the value of the component's Spec accessor, or ok=false
// when the component exposes no such accessor.
func reflectSpec(c Component) (any, bool) {
	m := reflect.ValueOf(c).MethodByName("Spec")
	if !m.IsValid() || m.Type().NumIn() != 0 || m.Type().NumOut() != 1 {
		return nil, false
	}

	return m.Call(nil)[0].Interface(), true
}

// reflectName calls a no-argument accessor (e.g. "Connection" or "Component")
// on obj and returns the Name of the returned value. ok is false when the
// accessor is absent, returns nil, or returns something without a Name.
func reflectName(obj any, method string) (string, bool) {
	m := reflect.ValueOf(obj).MethodByName(method)
	if !m.IsValid() || m.Type().NumIn() != 0 || m.Type().NumOut() != 1 {
		return "", false
	}

	out := m.Call(nil)[0]
	if (out.Kind() == reflect.Interface || out.Kind() == reflect.Pointer) && out.IsNil() {
		return "", false
	}

	n, ok := out.Interface().(named)
	if !ok {
		return "", false
	}

	return n.Name(), true
}
