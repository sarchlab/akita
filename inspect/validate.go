package inspect

import (
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

// validateSpecType mirrors modeling.ValidateSpec's structural checks without
// constructing a value or executing the inspected package's methods.
func validateSpecType(pkg *packages.Package, typ types.Type, index pkgIndex) error {
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("%s: Spec must be a struct, got %s", pkg.PkgPath, typ)
	}
	// Spec fields must follow the flat configuration rules even when the Spec
	// customizes its JSON.
	for i := range st.NumFields() {
		f := st.Field(i)
		if err := validateSpecFieldType(f.Type()); err != nil {
			return posErrorf(pkg, f.Pos(), "Spec field %s: %v", f.Name(), err)
		}
		if reflect.StructTag(st.Tag(i)).Get("json") == "-" {
			return posErrorf(pkg, f.Pos(), "Spec field spec.%s must not be tagged json:\"-\"", f.Name())
		}
		if !f.Exported() {
			return posErrorf(pkg, f.Pos(), "Spec field spec.%s must be exported", f.Name())
		}
		_, options, _ := strings.Cut(reflect.StructTag(st.Tag(i)).Get("json"), ",")
		if slices.Contains(strings.Split(options, ","), "omitzero") && hasCustomIsZero(f.Type()) {
			return posErrorf(pkg, f.Pos(), "Spec field spec.%s with custom IsZero must not use json:\",omitzero\"", f.Name())
		}
		if err := validateSpecJSONType(pkg, f.Type(), index); err != nil {
			return err
		}
	}
	return validateSpecJSONType(pkg, typ, index)
}

// validateSpecFieldType accepts scalar field types (booleans, integers except
// uintptr, floats, strings, and named types based on them) and slices and
// arrays of scalars, and slices of flat scalar records.
func validateSpecFieldType(typ types.Type) error {
	if isScalarType(typ) {
		return nil
	}

	switch t := typ.Underlying().(type) {
	case *types.Slice:
		if st, ok := t.Elem().Underlying().(*types.Struct); ok {
			return validateSpecRecordType(st)
		}
		if isScalarType(t.Elem()) {
			return nil
		}
	case *types.Array:
		if isScalarType(t.Elem()) {
			return nil
		}
	}

	return fmt.Errorf("disallowed Spec type %s: Spec fields must be scalars "+
		"or slices of scalars, or slices of flat scalar records", typ)
}

// isScalarType reports whether typ is a boolean, an integer other than
// uintptr, a float, a string, or a named type based on one.
func isScalarType(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)

	return ok && t.Kind() != types.Uintptr &&
		t.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) != 0
}

// validateFieldMetadata checks the Spec field metadata that the runtime does
// not re-check, so the inspector is where it is enforced: min and max apply
// only to numeric fields, and no two fields share a JSON name.
func validateFieldMetadata(pkg *packages.Package, specType types.Type, fields []schema.Field) error {
	st := specType.Underlying().(*types.Struct)
	seen := map[string]bool{}
	for _, field := range fields {
		var f *types.Var
		for candidate := range st.Fields() {
			if candidate.Name() == field.Name {
				f = candidate
				break
			}
		}
		if field.Min != nil || field.Max != nil {
			basic, ok := f.Type().Underlying().(*types.Basic)
			if !ok || basic.Info()&(types.IsInteger|types.IsFloat) == 0 || basic.Kind() == types.Uintptr {
				return posErrorf(pkg, f.Pos(), "Spec field %s: min/max apply only to numeric fields", f.Name())
			}
		}
		if field.JSONName == "" {
			continue
		}
		if seen[field.JSONName] {
			return posErrorf(pkg, f.Pos(), "Spec has duplicate JSON name %q", field.JSONName)
		}
		seen[field.JSONName] = true
	}
	return nil
}

func validateSpecJSONType(pkg *packages.Package, typ types.Type, index pkgIndex) error {
	if jsonPkg := index["encoding/json"]; jsonPkg != nil {
		marshaler := jsonPkg.Types.Scope().Lookup("Marshaler").Type().Underlying().(*types.Interface)
		unmarshaler := jsonPkg.Types.Scope().Lookup("Unmarshaler").Type().Underlying().(*types.Interface)
		if (types.Implements(typ, marshaler) || types.Implements(types.NewPointer(typ), marshaler)) &&
			!types.Implements(types.NewPointer(typ), unmarshaler) {
			return fmt.Errorf("%s: Spec type %s customizes MarshalJSON but has no UnmarshalJSON", pkg.PkgPath, typ)
		}
	}
	switch t := typ.Underlying().(type) {
	case *types.Slice:
		return validateSpecJSONType(pkg, t.Elem(), index)
	case *types.Array:
		return validateSpecJSONType(pkg, t.Elem(), index)
	case *types.Struct:
		for i := range t.NumFields() {
			if err := validateSpecJSONType(pkg, t.Field(i).Type(), index); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasCustomIsZero matches encoding/json's IsZero() bool method contract.
func hasCustomIsZero(typ types.Type) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(typ), false, nil, "IsZero")
	if obj == nil {
		return false
	}
	sig, ok := obj.Type().(*types.Signature)
	return ok && sig.Params().Len() == 0 && sig.Results().Len() == 1 &&
		types.Identical(sig.Results().At(0).Type(), types.Typ[types.Bool])
}

func validateSpecRecordType(st *types.Struct) error {
	seen := map[string]bool{}
	for i := range st.NumFields() {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		name, options, _ := strings.Cut(tag, ",")
		if !f.Exported() || !isScalarType(f.Type()) || name == "-" || !isValidJSONName(name) {
			return fmt.Errorf("record field %s must be an exported scalar with an explicit JSON name", f.Name())
		}
		if seen[name] {
			return fmt.Errorf("record has duplicate JSON name %q", name)
		}
		seen[name] = true
		if slices.Contains(strings.Split(options, ","), "omitzero") && hasCustomIsZero(f.Type()) {
			return fmt.Errorf("record field %s with custom IsZero must not use omitzero", f.Name())
		}
	}
	return nil
}
