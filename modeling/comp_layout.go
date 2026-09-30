package modeling

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var (
	portType       = reflect.TypeFor[messaging.Port]()
	portSliceType  = reflect.TypeFor[[]messaging.Port]()
	middlewareType = reflect.TypeFor[Middleware]()
	freqType       = reflect.TypeFor[timing.Freq]()
)

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
// every field is an exported messaging.Port (a port) or []messaging.Port (a
// port group).
func portLayoutOf(t reflect.Type) *portLayout {
	if l, ok := portLayouts.Load(t); ok {
		return l.(*portLayout)
	}

	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("modeling: Ports type %s must be a struct", t))
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

// middlewareList returns the fields of a Middlewares struct in declaration
// order. It panics unless every field is exported, implements Middleware, and
// is set.
func middlewareList(v reflect.Value) []Middleware {
	t := v.Type()
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("modeling: Middlewares type %s must be a struct", t))
	}

	out := make([]Middleware, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || !f.Type.Implements(middlewareType) {
			panic(fmt.Sprintf(
				"modeling: Middlewares field %s.%s must be exported and "+
					"implement modeling.Middleware", t, f.Name))
		}

		fv := v.Field(i)
		if isNilable(fv.Kind()) && fv.IsNil() {
			panic(fmt.Sprintf(
				"modeling: Middlewares field %s.%s is not set by NewMiddlewares",
				t, f.Name))
		}

		out = append(out, fv.Interface().(Middleware))
	}

	return out
}

func isNilable(k reflect.Kind) bool {
	switch k {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice,
		reflect.Func, reflect.Chan:
		return true
	default:
		return false
	}
}

// specFreq returns the Spec's Freq field, the clock the component ticks at.
func specFreq(spec any) timing.Freq {
	v := reflect.ValueOf(spec)
	f := v.FieldByName("Freq")
	if !f.IsValid() || f.Type() != freqType {
		panic(fmt.Sprintf(
			"modeling: Spec %s must have a Freq timing.Freq field", v.Type()))
	}

	return f.Interface().(timing.Freq)
}
