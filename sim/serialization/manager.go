package serialization

import (
	"fmt"
	"reflect"
	"sync"
)

type Codec interface {
	Encode(v map[string]any) error
	Decode() (map[string]any, error)
}

// Manager is a manager that can serialize and deserialize objects.
type Manager struct {
	codec Codec
	lock  sync.Mutex

	serialized map[string][]byte
}

func NewManager(codec Codec) *Manager {
	return &Manager{
		codec:      codec,
		lock:       sync.Mutex{},
		serialized: make(map[string][]byte),
	}
}

func (m *Manager) serializeToMap(
	obj any,
) (map[string]any, error) {
	typeName := reflect.TypeOf(obj).String()

	switch reflect.TypeOf(obj).Kind() {
	case reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		// Do nothing
		return map[string]any{
			"type":  typeName,
			"value": obj,
		}, nil

	case reflect.Ptr:
		return m.serializePtr(obj, typeName)

	case reflect.Slice:
		return m.serializeSlice(obj)

	case reflect.Map:
		return m.serializeMap(obj)

	case reflect.Struct:
		return m.serializeStruct(obj)
	}

	return nil, fmt.Errorf("unsupported type: %s", reflect.TypeOf(obj).String())
}

func (*Manager) serializeStruct(obj any) (map[string]any, error) {
	serializable, ok := obj.(Serializable)
	if !ok {
		return nil, fmt.Errorf(
			"%s is not a Serializable",
			reflect.TypeOf(obj).String(),
		)
	}

	mapped, err := serializable.Serialize()
	if err != nil {
		return nil, err
	}

	mapped["type"] = reflect.TypeOf(obj).String()

	return mapped, nil
}

func (*Manager) serializeMap(obj any) (map[string]any, error) {
	mapValue := reflect.ValueOf(obj)
	simpleMap := make(map[string]any, mapValue.Len())

	for _, key := range mapValue.MapKeys() {
		simpleMap[key.String()] = mapValue.MapIndex(key).Interface()
	}

	return map[string]any{
		"type":  "map",
		"value": simpleMap,
	}, nil
}

func (m *Manager) serializeSlice(obj any) (map[string]any, error) {
	slice := reflect.ValueOf(obj)
	simpleSlice := make([]interface{}, slice.Len())

	for i := 0; i < slice.Len(); i++ {
		element := slice.Index(i)

		simple, err := m.serializeToMap(element.Interface())
		if err != nil {
			return nil, err
		}

		simpleSlice[i] = simple
	}

	return map[string]any{
		"type":  "slice",
		"value": simpleSlice,
	}, nil
}

func (m *Manager) serializePtr(
	obj any,
	typeName string,
) (map[string]any, error) {
	if reflect.ValueOf(obj).IsNil() {
		return map[string]any{
			"type":  typeName,
			"value": nil,
		}, nil
	}

	nested, err := m.serializeToMap(
		reflect.ValueOf(obj).Elem().Interface(),
	)
	if err != nil {
		return nil, err
	}

	nested["is_ptr"] = true

	return nested, nil
}

func (m *Manager) Serialize(obj any) error {
	var err error

	m.lock.Lock()
	defer m.lock.Unlock()

	mapped, err := m.serializeToMap(obj)
	if err != nil {
		return err
	}

	err = m.codec.Encode(mapped)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) Deserialize(obj any) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	panic("not implemented")
}
