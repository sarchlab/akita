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
	if m.mode != modeSerialization {
		panic("serialization session is not active")
	}

	k, _ := typeKindName(obj)
	switch k {
	case "slice":
		return m.serializeSlice(obj)
	case "serializable":
		return m.serializeSerializable(obj.(Serializable))
	default:
		return m.serializePrimitive(obj)
	}
}

// Deserialize converts a Value to an object.
func (m *Manager) Deserialize(v *Value) (any, error) {
	switch v.K {
	case "id":
		return m.deserializeSerializable(v)
	case "slice":
		return m.deserializeSlice(v)
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

func (m *Manager) serializeSlice(obj any) (*Value, error) {
	slice := reflect.ValueOf(obj)
	values := make([]*Value, 0, slice.Len())

	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i)

		v, err := m.Serialize(elem.Interface())
		if err != nil {
			return nil, err
		}

		values = append(values, v)
	}

	k, t := typeKindName(obj)
	v := &Value{
		T: t,
		K: k,
		V: values,
	}

	return v, nil
}

func (m *Manager) deserializeSlice(v *Value) (any, error) {
	if v.K != "slice" {
		return nil, fmt.Errorf("value kind is %s, not 'slice'", v.K)
	}

	rawSlice, ok := v.V.([]any)
	if !ok {
		return nil, fmt.Errorf("value V is not a slice")
	}

	result := make([]any, 0, len(rawSlice))

	for _, elem := range rawSlice {
		elemMap, ok := elem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("slice element is not a map")
		}

		elemVal := v.mapToValue(elemMap)

		deserializedElem, err := m.Deserialize(elemVal)
		if err != nil {
			return nil, err
		}

		result = append(result, deserializedElem)
	}

	return result, nil
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
	kind = typeOf.Kind().String()

	switch kind {
	case "ptr":
		typeOf = typeOf.Elem()
	case "slice":
		kind = "slice"
		typeOf = typeOf.Elem()

		if typeOf.Kind() == reflect.Ptr {
			typeOf = typeOf.Elem()
		}
	}

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
