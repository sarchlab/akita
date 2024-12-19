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

func (m *Manager) Deserialize(s Serializable) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	mapped, err := m.codec.Decode()
	if err != nil {
		return err
	}

	deserializedMap, err := m.deserializeInternal(mapped)
	if err != nil {
		return err
	}

	err = s.Deserialize(deserializedMap.(map[string]any))
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) serializeToMap(
	obj any,
) (map[string]any, error) {
	objType := reflect.TypeOf(obj)

	switch objType.Kind() {
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
		return map[string]any{
			"type_kind": objType.Kind().String(),
			"value":     obj,
		}, nil

	case reflect.Ptr:
		return m.serializePtr(obj)

	case reflect.Slice:
		return m.serializeSlice(obj)

	case reflect.Map:
		return m.serializeMap(obj)
	}

	return nil, fmt.Errorf("unsupported type: %s", objType.String())
}

func (*Manager) serializeMap(obj any) (map[string]any, error) {
	mapValue := reflect.ValueOf(obj)
	simpleMap := make(map[string]any, mapValue.Len())

	for _, key := range mapValue.MapKeys() {
		simpleMap[key.String()] = mapValue.MapIndex(key).Interface()
	}

	return map[string]any{
		"value":     simpleMap,
		"type_kind": reflect.TypeOf(obj).Kind().String(),
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

	elemType := reflect.TypeOf(obj).Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	typeName := elemType.Name()
	if elemType.Kind() == reflect.Interface ||
		elemType.Kind() == reflect.Struct {
		typeName = elemType.PkgPath() + "." + typeName
	}

	return map[string]any{
		"value":     simpleSlice,
		"type":      typeName,
		"type_kind": reflect.TypeOf(obj).Kind().String(),
	}, nil
}

func (m *Manager) serializePtr(
	obj any,
) (map[string]any, error) {
	typeKind, typeName := typeKindName(obj)

	if typeKind == "nil" {
		return map[string]any{
			"type":      typeName,
			"type_kind": typeKind,
			"value":     nil,
		}, nil
	}

	if typeKind == "serializable" {
		simpleMap, err := obj.(Serializable).Serialize()
		if err != nil {
			return nil, err
		}

		simpleMap, err = m.serializeStructInternal(simpleMap)
		if err != nil {
			return nil, err
		}

		simpleMap["type"] = typeName
		simpleMap["type_kind"] = "serializable"

		return simpleMap, nil
	}

	nested := map[string]any{
		"type":      typeName,
		"type_kind": typeKind,
		"value":     nil,
	}

	value, err := m.serializeToMap(
		reflect.ValueOf(obj).Elem().Interface(),
	)
	if err != nil {
		return nil, err
	}

	nested["value"] = value

	return nested, nil
}

func (m *Manager) serializeStructInternal(
	mapped map[string]any,
) (map[string]any, error) {
	for k, v := range mapped {
		simple, err := m.serializeToMap(v)
		if err != nil {
			return nil, err
		}

		mapped[k] = simple
	}

	return mapped, nil
}

func (m *Manager) deserializeInternal(
	mapped map[string]any,
) (any, error) {
	typeKind := mapped["type_kind"].(string)

	switch typeKind {
	case "slice":
		return m.deserializeSlice(mapped)
	case "serializable":
		return m.deserializeSerializableInternal(mapped)
	default:
		return m.deserializePrimitive(mapped)
	}
}

func (m *Manager) deserializeSlice(mapped map[string]any) (any, error) {
	delete(mapped, "type")
	delete(mapped, "type_kind")

	slice := []any{}

	for _, v := range mapped["value"].([]any) {
		simple, err := m.deserializeInternal(v.(map[string]any))
		if err != nil {
			return nil, err
		}

		slice = append(slice, simple)
	}

	return slice, nil
}

func (m *Manager) deserializeSerializableInternal(
	mapped map[string]any,
) (any, error) {
	delete(mapped, "type")
	delete(mapped, "type_kind")

	for k, v := range mapped {
		simple, err := m.deserializeInternal(v.(map[string]any))
		if err != nil {
			return nil, err
		}

		mapped[k] = simple
	}

	return mapped, nil
}

// Type converter function type
type typeConverter func(any) any

// Map of type converters
var typeConverters = map[string]typeConverter{
	"bool":       func(v any) any { return v.(bool) },
	"int":        func(v any) any { return int(v.(float64)) },
	"int8":       func(v any) any { return int8(v.(float64)) },
	"int16":      func(v any) any { return int16(v.(float64)) },
	"int32":      func(v any) any { return int32(v.(float64)) },
	"int64":      func(v any) any { return int64(v.(float64)) },
	"uint":       func(v any) any { return uint(v.(float64)) },
	"uint8":      func(v any) any { return uint8(v.(float64)) },
	"uint16":     func(v any) any { return uint16(v.(float64)) },
	"uint32":     func(v any) any { return uint32(v.(float64)) },
	"uint64":     func(v any) any { return uint64(v.(float64)) },
	"float32":    func(v any) any { return float32(v.(float64)) },
	"float64":    func(v any) any { return v.(float64) },
	"ptr":        func(v any) any { return v },
	"string":     func(v any) any { return v },
	"complex64":  func(v any) any { return complex64(v.(complex128)) },
	"complex128": func(v any) any { return v.(complex128) },
	"nil":        func(v any) any { return nil },
}

func (m *Manager) deserializePrimitive(mapValue map[string]any) (any, error) {
	typeKind := mapValue["type_kind"].(string)
	converter, exists := typeConverters[typeKind]

	if !exists {
		return nil, fmt.Errorf(
			"type %s is not supported or not primitive",
			typeKind,
		)
	}

	return converter(mapValue["value"]), nil
}

func typeKindName(val any) (kind, name string) {
	if reflect.ValueOf(val).IsNil() {
		return "nil", "nil"
	}

	typeOf := reflect.TypeOf(val)
	if typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}

	kind = typeOf.Kind().String()
	name = typeOf.PkgPath() + "." + typeOf.Name()

	if registered(name) {
		kind = "serializable"
	}

	return kind, name
}

func registered(typeName string) bool {
	registeredType := registry.registeredType(typeName)

	return registeredType != nil
}
