// Package valuecheck validates value types used in checkpoints and payloads.
package valuecheck

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// jsonMarshalerType is the reflect.Type of json.Marshaler, used to exempt types
// that customize their own JSON from the structural and data-loss checks.
var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// jsonUnmarshalerType is the reflect.Type of json.Unmarshaler. A type that
// customizes MarshalJSON must also customize UnmarshalJSON: round-tripping is
// two independent mechanisms, and with only the marshal half a checkpoint
// saves the custom payload and then silently restores zero values (the
// default decoder only sets exported fields).
var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

func (w *walker) validateValue(v reflect.Value, path string, allowComposite bool) error {
	if !v.IsValid() {
		return fmt.Errorf("%s: invalid value", path)
	}

	t := v.Type()

	if t.Kind() != reflect.Struct {
		return fmt.Errorf("%s: expected struct, got %s", path, t.Kind())
	}

	return w.validateStructType(t, path, allowComposite)
}

func (w *walker) validateFieldType(t reflect.Type, path string, allowComposite bool) error {
	if t == w.opaque {
		return nil
	}
	if w.seen[t] {
		return nil
	}
	w.seen[t] = true

	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return nil

	case reflect.Slice, reflect.Array:
		if !allowComposite {
			if !isScalarKind(t.Elem().Kind()) {
				return fmt.Errorf("%s: %s of %s not allowed in spec; "+
					"spec fields must be scalars or slices of scalars",
					path, t.Kind(), t.Elem().Kind())
			}

			return nil
		}

		return w.validateFieldType(t.Elem(), path+"[]", allowComposite)

	case reflect.Map:
		if !allowComposite {
			return fmt.Errorf("%s: map not allowed in spec; "+
				"spec fields must be scalars or slices of scalars", path)
		}

		k := t.Key().Kind()
		if k != reflect.String &&
			k != reflect.Uint64 && k != reflect.Uint && k != reflect.Uint32 &&
			k != reflect.Uint8 && k != reflect.Uint16 &&
			k != reflect.Int8 && k != reflect.Int16 &&
			k != reflect.Int64 && k != reflect.Int && k != reflect.Int32 {
			return fmt.Errorf("%s: map key must be string or integer, got %s", path, k)
		}

		return w.validateFieldType(t.Elem(), path+"[value]", allowComposite)

	case reflect.Struct:
		if !allowComposite {
			return fmt.Errorf("%s: nested structs not allowed in spec", path)
		}

		// Validate the nested struct's fields recursively.
		return w.validateStructType(t, path, allowComposite)

	case reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return fmt.Errorf("%s: disallowed kind %s", path, t.Kind())

	default:
		return fmt.Errorf("%s: unsupported kind %s", path, t.Kind())
	}
}

// isScalarKind reports whether k is a scalar kind: a boolean, an integer, a
// float, or a string.
func isScalarKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	default:
		return false
	}
}

func (w *walker) validateStructType(t reflect.Type, path string, allowComposite bool) error {
	// Spec fields must be scalars or slices of scalars even when the Spec
	// customizes its JSON: any other container is not configuration.
	if !allowComposite {
		if err := w.validateFields(t, path, allowComposite); err != nil {
			return err
		}
	}

	// A type that customizes its own JSON is otherwise trusted: it round-trips
	// on its own terms, so the remaining structural rules and the data-loss
	// guard do not apply — provided both halves of the round trip exist. UnmarshalJSON must be
	// checked on the pointer type: it mutates the value, so it always has a
	// pointer receiver, while MarshalJSON typically has a value receiver.
	if t.Implements(jsonMarshalerType) {
		if !reflect.PointerTo(t).Implements(jsonUnmarshalerType) {
			return fmt.Errorf(
				"%s: type %s customizes MarshalJSON but has no UnmarshalJSON, "+
					"so a checkpoint saves its custom payload and then silently "+
					"restores zero values (the default decoder only sets "+
					"exported fields); implement the pair",
				path, t)
		}

		if w.strict {
			return w.validateFields(t, path, allowComposite)
		}
		return nil
	}

	// Data-loss guard: a struct whose state is entirely unexported, with no
	// MarshalJSON, serializes as {} and silently drops its contents across a
	// checkpoint (the lruset.Set class of bug). Catch it at validation time
	// instead of as a wrong resume.
	if serializesToEmpty(t) {
		return fmt.Errorf(
			"%s: type %s has unexported state but no MarshalJSON, so it "+
				"serializes as {} and would silently lose that state across a "+
				"checkpoint; add MarshalJSON/UnmarshalJSON or export the fields",
			path, t)
	}

	if err := checkDuplicateJSONNames(t, path); err != nil {
		return err
	}

	if allowComposite {
		return w.validateFields(t, path, allowComposite)
	}

	return nil
}

// validateFields checks every field's type against the Spec or State rules.
func (w *walker) validateFields(t reflect.Type, path string, allowComposite bool) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// State may exempt a field that setup rebuilds; every Spec field
		// must still be a scalar.
		if tag := field.Tag.Get("json"); tag == "-" && allowComposite && !w.strict {
			continue
		}

		fieldPath := fmt.Sprintf("%s.%s", path, field.Name)

		if err := w.validateFieldType(field.Type, fieldPath, allowComposite); err != nil {
			return err
		}
	}

	return nil
}

// checkDuplicateJSONNames rejects exported fields that encode under the same
// JSON name, following encoding/json's rules for embedded structs. When names
// collide, encoding/json keeps at most one of the fields, so a checkpoint
// would silently lose the others.
func checkDuplicateJSONNames(t reflect.Type, path string) error {
	fields := map[string][]string{}
	collectJSONFields(t, "", fields)

	var dups []string
	for name, paths := range fields {
		if len(paths) > 1 {
			dups = append(dups, name)
		}
	}

	if len(dups) == 0 {
		return nil
	}

	slices.Sort(dups)
	name := dups[0]

	return fmt.Errorf(
		"%s: fields %s share JSON name %q; encoding/json keeps at most one "+
			"of them, so a checkpoint would lose the others",
		path, strings.Join(fields[name], " and "), name)
}

// collectJSONFields records, by JSON name, the Go path of every field
// encoding/json encodes for t. Like encoding/json, it flattens embedded
// structs whose JSON tag gives no valid name, including unexported ones, and
// encodes an unexported embedded struct that the tag does name.
func collectJSONFields(t reflect.Type, prefix string, fields map[string][]string) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		if !isValidJSONName(name) {
			name = ""
		}

		isStruct := f.Type.Kind() == reflect.Struct

		switch {
		case f.Anonymous && isStruct && name == "":
			collectJSONFields(f.Type, prefix+f.Name+".", fields)
			continue
		case f.Anonymous && isStruct:
			// A named embedded struct is encoded even when unexported.
		case !f.IsExported():
			continue
		}

		if name == "" {
			name = f.Name
		}

		fields[name] = append(fields[name], prefix+f.Name)
	}
}

// isValidJSONName reports whether encoding/json accepts name from a struct
// tag. It falls back to the field name otherwise. Letters, digits, and most
// punctuation are allowed; backslashes and quotes are reserved.
func isValidJSONName(name string) bool {
	if name == "" {
		return false
	}

	for _, c := range name {
		if strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) {
			continue
		}

		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}

	return true
}

// serializesToEmpty reports whether a struct type holds unexported fields yet
// marshals to an empty JSON object — meaning encoding/json silently drops all of
// it. Types that implement json.Marshaler are handled by the caller and never
// reach here. The partial case (some exported, some unexported fields) is
// deliberately not flagged: an unexported field there may be intentional
// rebuilt-on-load scratch, which is ambiguous, so it stays a review concern.
func serializesToEmpty(t reflect.Type) bool {
	hasUnexported := false
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).PkgPath != "" { // unexported field
			hasUnexported = true
			break
		}
	}
	if !hasUnexported {
		return false
	}

	data, err := json.Marshal(reflect.New(t).Elem().Interface())
	return err == nil && string(data) == "{}"
}

type walker struct {
	strict bool
	opaque reflect.Type
	seen   map[reflect.Type]bool
}

// Spec validates a component configuration.
func Spec(v any) error {
	return (&walker{seen: map[reflect.Type]bool{}}).validateValue(reflect.ValueOf(v), "spec", false)
}

// State validates a component state, trusting paired custom JSON methods.
func State(v any) error {
	return (&walker{seen: map[reflect.Type]bool{}}).validateValue(reflect.ValueOf(v), "state", true)
}

// Payload validates every field, including JSON-excluded and custom-encoded
// fields. Only opaque (the self-serializing message envelope) is exempt.
func Payload(t, opaque reflect.Type) error {
	if t == nil {
		return fmt.Errorf("payload: invalid nil type")
	}
	return (&walker{strict: true, opaque: opaque, seen: map[reflect.Type]bool{}}).validateFieldType(t, "payload", true)
}
