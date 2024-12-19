package serialization

import (
	"fmt"
	"reflect"
	"sync"
)

type typeRegistry struct {
	lock sync.RWMutex

	types map[string]reflect.Type
}

func (r *typeRegistry) registerType(
	t reflect.Type,
) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	tt := t
	if tt.Kind() == reflect.Ptr {
		tt = t.Elem()
	}

	typeName := tt.PkgPath() + "." + tt.Name()

	if _, ok := r.types[typeName]; ok {
		return fmt.Errorf("type %s already registered", typeName)
	}

	r.types[typeName] = t

	return nil
}

func (r *typeRegistry) createInstance(typeName string) (any, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	exampleType, ok := r.types[typeName]
	if !ok {
		return nil, fmt.Errorf("type %s not found", typeName)
	}

	valueType := exampleType
	if exampleType.Kind() == reflect.Ptr {
		valueType = exampleType.Elem()
	}

	newValue := reflect.New(valueType)
	if exampleType.Kind() != reflect.Ptr {
		newValue = newValue.Elem()
	}

	return newValue.Interface(), nil
}

func (r *typeRegistry) registeredType(typeName string) reflect.Type {
	r.lock.RLock()
	defer r.lock.RUnlock()

	t, ok := r.types[typeName]
	if !ok {
		return nil
	}

	return t
}

var registry = typeRegistry{
	types: map[string]reflect.Type{
		"bool":       reflect.TypeOf(bool(false)),
		"int":        reflect.TypeOf(int(0)),
		"int8":       reflect.TypeOf(int8(0)),
		"int16":      reflect.TypeOf(int16(0)),
		"int32":      reflect.TypeOf(int32(0)),
		"int64":      reflect.TypeOf(int64(0)),
		"uint":       reflect.TypeOf(uint(0)),
		"uint8":      reflect.TypeOf(uint8(0)),
		"uint16":     reflect.TypeOf(uint16(0)),
		"uint32":     reflect.TypeOf(uint32(0)),
		"uint64":     reflect.TypeOf(uint64(0)),
		"uintptr":    reflect.TypeOf(uintptr(0)),
		"float32":    reflect.TypeOf(float32(0)),
		"float64":    reflect.TypeOf(float64(0)),
		"complex64":  reflect.TypeOf(complex64(0)),
		"complex128": reflect.TypeOf(complex128(0)),
		"string":     reflect.TypeOf(string("")),
	},
}

func RegisterType(t reflect.Type) error {
	return registry.registerType(t)
}
