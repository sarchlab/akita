package blueprint

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// Instantiate creates an expanded declaration in the supplied simulation.
// Simulation.Initialize calls this before setup validation. It is an assembly
// hook, not a replacement for Simulation.Initialize. Failures panic; discard the
// simulation after any failure. The returned map contains only named exported
// Ports values; live entities are registered in the Simulation inventory.
func (bp *Blueprint) Instantiate(s modeling.Simulation) map[string]any {
	if !bp.expanded {
		failf("", "Instantiate", "call Expand first")
	}
	objects := make(map[string]any)
	resources := make(map[string]*ResourceNode)
	for _, e := range bp.entries {
		if n, ok := e.node.(*ResourceNode); ok {
			resources[e.node.Name()] = n
		}
	}
	for _, name := range bp.resourceOrder(resources) {
		n := resources[name]
		contextual(name, "resource", func() {
			objects[name] = n.create(s, name, clone(n.Spec), inject(n.resourcesType, n.Uses, objects))
		})
	}
	bp.instantiateComponents(s, objects)
	bp.instantiateConnections(s, objects)
	return bp.instantiateExportedPorts(objects)
}

func (bp *Blueprint) instantiateComponents(s modeling.Simulation, objects map[string]any) {
	for _, e := range bp.entries {
		n, ok := e.node.(*ComponentNode)
		if !ok {
			continue
		}
		contextual(e.node.Name(), "component", func() {
			c := n.create(s, e.node.Name(), clone(n.Spec), inject(n.resourcesType, n.Uses, objects))
			objects[e.node.Name()] = c
			for _, p := range n.Ports {
				port := twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(p.Spec).Build()
				c.BindPort(p.Slot, port)
				objects[join(e.node.Name(), p.Slot)] = port
			}
		})
	}
}

func (bp *Blueprint) instantiateConnections(s modeling.Simulation, objects map[string]any) {
	for _, e := range bp.entries {
		contextual(e.node.Name(), "connection", func() {
			switch n := e.node.(type) {
			case *ConnectionNode:
				c := direct.NewConnection(e.node.Name(), s, n.Freq)
				for _, ref := range n.Members {
					c.BindPort(objects[string(ref)].(messaging.Port))
				}
				objects[e.node.Name()] = c
			case *NetworkNode:
				ports := make([]messaging.Port, len(n.members))
				for i, ref := range n.members {
					ports[i] = objects[string(ref)].(messaging.Port)
				}
				n.assemble(clone(n.spec), s, e.node.Name(), ports)
			}
		})
	}
}

func (bp *Blueprint) instantiateExportedPorts(objects map[string]any) map[string]any {
	exports := make(map[string]any, len(bp.exports))
	for _, export := range bp.exports {
		value := reflect.New(export.portsType).Elem()
		for _, binding := range export.bindings {
			setPort(value, binding.field, objects[binding.target].(messaging.Port))
		}
		exports[export.name] = value.Interface()
	}
	return exports
}

func inject(t reflect.Type, uses map[string]string, objects map[string]any) reflect.Value {
	v := reflect.New(t).Elem()
	for i := range t.NumField() {
		f := t.Field(i)
		v.Field(i).Set(reflect.ValueOf(objects[uses[f.Name]]))
	}
	return v
}

func setPort(v reflect.Value, slot string, p messaging.Port) {
	bracket := strings.IndexByte(slot, '[')
	if bracket < 0 {
		v.FieldByName(slot).Set(reflect.ValueOf(p))
		return
	}
	field := v.FieldByName(slot[:bracket])
	index, err := strconv.Atoi(slot[bracket+1 : len(slot)-1])
	if err != nil {
		panic(err)
	}
	if field.Len() <= index {
		field.Grow(index + 1 - field.Len())
		field.SetLen(index + 1)
	}
	field.Index(index).Set(reflect.ValueOf(p))
}
