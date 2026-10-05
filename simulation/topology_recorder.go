package simulation

import (
	"encoding/json"
	"reflect"

	"github.com/sarchlab/akita/v5/simulation/datarecording"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/naming"
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

// topologyRecorder records the static structure of a simulation — every
// component's spec and the full port inventory with its connection graph — into
// the recording, making it self-describing for tools such as Daisen's index
// page.
//
// Specs are read from the exported Spec field that every component built from
// a component model has; because its type differs per component there is no
// single non-generic interface to assert against, so the recorder reaches it by
// reflection. This runs once at Terminate, so its cost is irrelevant to
// simulation speed.
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
		entry := portEntry{Port: p.Name()}

		if mp, ok := p.(messaging.Port); ok {
			if owner, ok := mp.Owner().(naming.Named); ok {
				entry.Component = owner.Name()
			}

			if conn := mp.Connection(); conn != nil {
				entry.Connection = conn.Name()
			}
		}

		r.recorder.InsertData(portTableName, entry)
	}
}

// reflectSpec returns the value of the component's exported Spec field, or
// ok=false when the component has no such field.
func reflectSpec(c Component) (any, bool) {
	v := reflect.Indirect(reflect.ValueOf(c))
	if v.Kind() != reflect.Struct {
		return nil, false
	}

	f := v.FieldByName("Spec")
	if !f.IsValid() || !f.CanInterface() {
		return nil, false
	}

	spec := f.Interface()
	if spec == nil {
		// A nil interface spec would make reflect.TypeOf(spec).String() panic
		// in the Terminate teardown path; treat it as "no spec".
		return nil, false
	}

	return spec, true
}
