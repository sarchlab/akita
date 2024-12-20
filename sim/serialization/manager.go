package serialization

import (
	"fmt"
	"io"
	"reflect"
	"sync"
)

// Value is a value that can be serialized.
type Value struct {
	T string `json:"T"`
	K string `json:"K"`
	V any    `json:"V"`
}

func (v *Value) mapToValue(m map[string]any) *Value {
	return &Value{
		T: m["T"].(string),
		K: m["K"].(string),
		V: m["V"],
	}
}

func nilValue() *Value {
	return &Value{
		T: "nil",
		K: "nil",
		V: nil,
	}
}

func IDToDeserialize(id string) *Value {
	return &Value{
		K: "id",
		V: id,
	}
}

type Codec interface {
	Encode(v map[string]*Value, writer io.Writer) error
	Decode(reader io.Reader) (map[string]*Value, error)
}

type mode int

const (
	modeNone mode = iota
	modeSerialization
	modeDeserialization
)

// Manager is a manager that can serialize and deserialize objects.
type Manager struct {
	codec Codec
	lock  sync.Mutex

	mode       mode
	data       map[string]*Value
	serialized map[string]any
}

func NewManager(codec Codec) *Manager {
	return &Manager{
		codec: codec,
		lock:  sync.Mutex{},
		mode:  modeNone,
	}
}

// StartSerialization starts a serialization session.
func (m *Manager) StartSerialization() {
	m.mode = modeSerialization

	m.data = make(map[string]*Value)
	m.serialized = make(map[string]any)
}

// StartDeserialization starts a deserialization session.
func (m *Manager) StartDeserialization(reader io.Reader) {
	m.mode = modeDeserialization
	m.serialized = make(map[string]any)

	var err error

	m.data, err = m.codec.Decode(reader)
	if err != nil {
		panic(err)
	}
}

// RegisterDeserializationStartingPoint registers a starting point for
// deserialization. This function allows merging deserialization with
// existing data.
func (m *Manager) RegisterDeserializationStartingPoint(s Serializable) {
	if m.mode != modeDeserialization {
		panic("deserialization session is not active")
	}

	m.registerDeserializationElement(s)
}

func (m *Manager) registerDeserializationElement(s Serializable) {
	m.serialized[s.Name()] = s

	value := reflect.ValueOf(s)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)

		_, typeName := typeKindName(field.Interface())
		if registered(typeName) {
			m.registerDeserializationElement(field.Interface().(Serializable))
		}
	}
}

// FinalizeSerialization finalizes a serialization session by writing the data
// to the writer.
func (m *Manager) FinalizeSerialization(writer io.Writer) {
	m.codec.Encode(m.data, writer)
	m.mode = modeNone
	m.data = nil
	m.serialized = nil
}

// FinalizeDeserialization finalizes a deserialization session. While calling
// this method is optional, it frees temporary memory allocated for the
// deserialization session.
func (m *Manager) FinalizeDeserialization() {
	m.mode = modeNone
	m.data = nil
	m.serialized = nil
}

// Serialize converts an object to a map[string]any.
func (m *Manager) Serialize(obj any) (*Value, error) {
	s, isSerializable := obj.(Serializable)
	if !isSerializable {
		v, err := m.serializePrimitive(obj)
		if err != nil {
			return nil, err
		}

		return v, nil
	}

	return m.serializeSerializable(s)
}

// Deserialize converts a Value to an object.
func (m *Manager) Deserialize(v *Value) (any, error) {
	switch v.K {
	case "id":
		return m.deserializeSerializable(v)
	default:
		return m.deserializePrimitive(v)
	}
}

func (m *Manager) serializePrimitive(obj any) (*Value, error) {
	k, t := typeKindName(obj)

	v := &Value{
		T: t,
		K: k,
		V: obj,
	}

	return v, nil
}

func (m *Manager) serializeSerializable(s Serializable) (*Value, error) {
	if s == nil || reflect.ValueOf(s).IsNil() {
		return nilValue(), nil
	}

	id := s.Name()
	if _, ok := m.serialized[id]; ok {
		return IDToDeserialize(id), nil
	}

	k, t := typeKindName(s)

	valMap, err := s.Serialize()
	if err != nil {
		return nil, err
	}

	for k, v := range valMap {
		v, err := m.Serialize(v)
		if err != nil {
			return nil, err
		}

		valMap[k] = v
	}

	v := &Value{
		T: t,
		K: k,
		V: valMap,
	}

	m.data[id] = v
	m.serialized[id] = v

	return IDToDeserialize(s.Name()), nil
}

// // Serialize adds an object to the serialization session.
// func (m *Manager) Serialize(obj any) error {
// 	if m.mode != modeSerialization {
// 		return fmt.Errorf("serialization session is not active")
// 	}

// 	var err error

// 	m.lock.Lock()
// 	defer m.lock.Unlock()

// 	mapped, err := m.serializeToMap(obj)
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (m *Manager) Deserialize(s Serializable) error {
// 	m.lock.Lock()
// 	defer m.lock.Unlock()

// 	mapped, err := m.codec.Decode()
// 	if err != nil {
// 		return err
// 	}

// 	deserializedMap, err := m.deserializeInternal(mapped)
// 	if err != nil {
// 		return err
// 	}

// 	err = s.Deserialize(deserializedMap.(map[string]any))
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// // DeserializeItem deserializes an item from a map.
// func (m *Manager) DeserializeItem(
// 	mapped map[string]any,
// ) (any, error) {
// 	typeKind := mapped["type_kind"].(string)

// 	switch typeKind {
// 	case "slice":
// 		return m.deserializeSlice(mapped)
// 	case "serializable":
// 		return m.deserializeSerializableInternal(mapped)
// 	default:
// 		return m.deserializePrimitive(mapped)
// 	}
// }

// func (m *Manager) serializeToMap(
// 	obj any,
// ) (map[string]any, error) {
// 	objType := reflect.TypeOf(obj)

// 	switch objType.Kind() {
// 	case reflect.Bool,
// 		reflect.Int,
// 		reflect.Int8,
// 		reflect.Int16,
// 		reflect.Int32,
// 		reflect.Int64,
// 		reflect.Uint,
// 		reflect.Uint8,
// 		reflect.Uint16,
// 		reflect.Uint32,
// 		reflect.Uint64,
// 		reflect.Uintptr,
// 		reflect.Float32,
// 		reflect.Float64,
// 		reflect.Complex64,
// 		reflect.Complex128,
// 		reflect.String:
// 		return map[string]any{
// 			"type_kind": objType.Kind().String(),
// 			"value":     obj,
// 		}, nil

// 	case reflect.Ptr:
// 		return m.serializePtr(obj)

// 	case reflect.Slice:
// 		return m.serializeSlice(obj)

// 	case reflect.Map:
// 		return m.serializeMap(obj)
// 	}

// 	return nil, fmt.Errorf("unsupported type: %s", objType.String())
// }

// func (*Manager) serializeMap(obj any) (map[string]any, error) {
// 	mapValue := reflect.ValueOf(obj)
// 	simpleMap := make(map[string]any, mapValue.Len())

// 	for _, key := range mapValue.MapKeys() {
// 		simpleMap[key.String()] = mapValue.MapIndex(key).Interface()
// 	}

// 	return map[string]any{
// 		"value":     simpleMap,
// 		"type_kind": reflect.TypeOf(obj).Kind().String(),
// 	}, nil
// }

// func (m *Manager) serializeSlice(obj any) (map[string]any, error) {
// 	slice := reflect.ValueOf(obj)
// 	simpleSlice := make([]interface{}, slice.Len())

// 	for i := 0; i < slice.Len(); i++ {
// 		element := slice.Index(i)

// 		simple, err := m.serializeToMap(element.Interface())
// 		if err != nil {
// 			return nil, err
// 		}

// 		simpleSlice[i] = simple
// 	}

// 	elemType := reflect.TypeOf(obj).Elem()
// 	if elemType.Kind() == reflect.Ptr {
// 		elemType = elemType.Elem()
// 	}

// 	typeName := elemType.Name()
// 	if elemType.Kind() == reflect.Interface ||
// 		elemType.Kind() == reflect.Struct {
// 		typeName = elemType.PkgPath() + "." + typeName
// 	}

// 	return map[string]any{
// 		"value":     simpleSlice,
// 		"type":      typeName,
// 		"type_kind": reflect.TypeOf(obj).Kind().String(),
// 	}, nil
// }

// func (m *Manager) serializePtr(
// 	obj any,
// ) (map[string]any, error) {
// 	typeKind, typeName := typeKindName(obj)

// 	if typeKind == "nil" {
// 		return map[string]any{
// 			"type":      typeName,
// 			"type_kind": typeKind,
// 			"value":     nil,
// 		}, nil
// 	}

// 	if typeKind == "serializable" {
// 		simpleMap, err := obj.(Serializable).Serialize()
// 		if err != nil {
// 			return nil, err
// 		}

// 		simpleMap, err = m.serializeStructInternal(simpleMap)
// 		if err != nil {
// 			return nil, err
// 		}

// 		simpleMap["type"] = typeName
// 		simpleMap["type_kind"] = "serializable"

// 		return simpleMap, nil
// 	}

// 	nested := map[string]any{
// 		"type":      typeName,
// 		"type_kind": typeKind,
// 		"value":     nil,
// 	}

// 	value, err := m.serializeToMap(
// 		reflect.ValueOf(obj).Elem().Interface(),
// 	)
// 	if err != nil {
// 		return nil, err
// 	}

// 	nested["value"] = value

// 	return nested, nil
// }

// func (m *Manager) serializeStructInternal(
// 	mapped map[string]any,
// ) (map[string]any, error) {
// 	for k, v := range mapped {
// 		simple, err := m.serializeToMap(v)
// 		if err != nil {
// 			return nil, err
// 		}

// 		mapped[k] = simple
// 	}

// 	return mapped, nil
// }

// func (m *Manager) deserializeInternal(
// 	mapped map[string]any,
// ) (any, error) {
// 	typeKind := mapped["type_kind"].(string)

// 	switch typeKind {
// 	case "slice":
// 		return m.deserializeSlice(mapped)
// 	case "serializable":
// 		return m.deserializeSerializableInternal(mapped)
// 	default:
// 		return m.deserializePrimitive(mapped)
// 	}
// }

// func (m *Manager) deserializeSlice(mapped map[string]any) (any, error) {
// 	delete(mapped, "type")
// 	delete(mapped, "type_kind")

// 	slice := []any{}

// 	for _, v := range mapped["value"].([]any) {
// 		simple, err := m.deserializeInternal(v.(map[string]any))
// 		if err != nil {
// 			return nil, err
// 		}

// 		slice = append(slice, simple)
// 	}

// 	return slice, nil
// }

// func (m *Manager) deserializeSerializableInternal(
// 	mapped map[string]any,
// ) (any, error) {
// 	delete(mapped, "type")
// 	delete(mapped, "type_kind")

// 	for k, v := range mapped {
// 		simple, err := m.deserializeInternal(v.(map[string]any))
// 		if err != nil {
// 			return nil, err
// 		}

// 		mapped[k] = simple
// 	}

// 	return mapped, nil
// }

func (m *Manager) deserializeSerializable(vID *Value) (any, error) {
	res, found := m.serialized[vID.V.(string)]
	if found {
		return res, nil
	}

	v, found := m.data[vID.V.(string)]
	if !found {
		return nil, fmt.Errorf("serialized object not found")
	}

	deserialized, err := registry.createInstance(v.T)
	if err != nil {
		return nil, err
	}

	deserializedMap, ok := v.V.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid serialized map")
	}

	for k, vMapIn := range deserializedMap {
		vIn := v.mapToValue(vMapIn.(map[string]any))

		deserializedIn, errIn := m.Deserialize(vIn)
		if errIn != nil {
			return nil, errIn
		}

		deserializedMap[k] = deserializedIn
	}

	err = deserialized.(Serializable).Deserialize(deserializedMap)
	if err != nil {
		return nil, err
	}

	m.serialized[deserialized.(Serializable).Name()] = deserialized

	return deserialized, nil
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

func (m *Manager) deserializePrimitive(v *Value) (any, error) {
	typeKind := v.K
	converter, exists := typeConverters[typeKind]

	if !exists {
		return nil, fmt.Errorf(
			"type %s is not supported or not primitive",
			typeKind,
		)
	}

	return converter(v.V), nil
}

func typeKindName(val any) (kind, name string) {
	typeOf := reflect.TypeOf(val)
	if typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}

	kind = typeOf.Kind().String()

	name = typeOf.Name()
	if typeOf.PkgPath() != "" {
		name = typeOf.PkgPath() + "." + name
	}

	if _, ok := val.(Serializable); ok {
		kind = "serializable"
	}

	return kind, name
}

func registered(typeName string) bool {
	registeredType := registry.registeredType(typeName)

	return registeredType != nil
}
