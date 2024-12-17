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

func (m *Manager) Deserialize() (any, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	mapped, err := m.codec.Decode()
	if err != nil {
		return nil, err
	}

	return m.deserializeFromMap(mapped)
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

	case reflect.Struct:
		return m.serializeStruct(obj)
	}

	return nil, fmt.Errorf("unsupported type: %s", objType.String())
}

func (m *Manager) serializeStruct(obj any) (map[string]any, error) {
	objType := reflect.TypeOf(obj)
	typeName := objType.PkgPath() + "." + objType.Name()

	serializable, ok := obj.(Serializable)
	if !ok {
		return nil, fmt.Errorf(
			"%s is not a Serializable",
			typeName,
		)
	}

	mapped, err := serializable.Serialize()
	if err != nil {
		return nil, err
	}

	mapped, err = m.serializeStructInternal(mapped)
	if err != nil {
		return nil, err
	}

	mapped["type"] = typeName
	mapped["type_kind"] = objType.Kind().String()

	return mapped, nil
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

	return map[string]any{
		"value":     simpleSlice,
		"type_kind": reflect.TypeOf(obj).Kind().String(),
	}, nil
}

func (m *Manager) serializePtr(
	obj any,
) (map[string]any, error) {
	objType := reflect.ValueOf(obj).Elem().Type()
	typeName := objType.PkgPath() + "." + objType.Name()

	if reflect.ValueOf(obj).IsNil() {
		return map[string]any{
			"type":      typeName,
			"type_kind": reflect.TypeOf(obj).Kind().String(),
			"value":     nil,
		}, nil
	}

	if registeredAsPtr(typeName) {
		simpleMap, err := obj.(Serializable).Serialize()
		if err != nil {
			return nil, err
		}

		simpleMap, err = m.serializeStructInternal(simpleMap)
		if err != nil {
			return nil, err
		}

		simpleMap["type"] = typeName
		simpleMap["type_kind"] = reflect.TypeOf(obj).Kind().String()

		return simpleMap, nil
	}

	nested := map[string]any{
		"type":      typeName,
		"type_kind": reflect.TypeOf(obj).Kind().String(),
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

func (m *Manager) deserializeFromMap(mapped map[string]any) (any, error) {
	typeKind := mapped["type_kind"].(string)

	switch typeKind {
	case "int":
		return m.deserializeInt(mapped)
	case "ptr":
		return m.deserializePtr(mapped)
	case "struct":
		return m.deserializeStruct(mapped)
	default:
		return nil, fmt.Errorf("unsupported type: %s", typeKind)
	}
}

func (m *Manager) deserializePtr(mapped map[string]any) (any, error) {
	typeName := mapped["type"].(string)

	if registeredAsPtr(typeName) {
		return m.deserializeStruct(mapped)
		// val, err := CreateInstance(typeName)
		// if err != nil {
		// 	return nil, err
		// }

		// value, err := val.Deserialize(mapped)
		// if err != nil {
		// 	return nil, err
		// }

		// return value, nil
	}

	rawValue, ok := mapped["value"]
	if !ok {
		return nil, fmt.Errorf("missing value field for ptr")
	}

	if rawValue == nil {
		return nil, nil
	}

	valueMap, ok := rawValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("value for ptr is not a map")
	}

	value, err := m.deserializeFromMap(valueMap)
	if err != nil {
		return nil, err
	}

	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Ptr {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)

		return ptr.Interface(), nil
	}

	return value, nil
}

func (m *Manager) deserializeStruct(mapped map[string]any) (any, error) {
	typeName := mapped["type"].(string)

	delete(mapped, "type")
	delete(mapped, "type_kind")

	for k, v := range mapped {
		nested, err := m.deserializeFromMap(v.(map[string]any))
		if err != nil {
			return nil, err
		}

		mapped[k] = nested
	}

	emptyV, err := CreateInstance(typeName)
	if err != nil {
		return nil, err
	}

	value, err := emptyV.Deserialize(mapped)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (*Manager) deserializeInt(mapped map[string]any) (any, error) {
	f64, ok := mapped["value"].(float64)
	if !ok {
		return nil, fmt.Errorf("value is not an int")
	}

	return int(f64), nil
}

func registeredAsPtr(typeName string) bool {
	registeredType := registry.registeredType(typeName)

	if registeredType == nil {
		return false
	}

	return registeredType.Kind() == reflect.Ptr
}
