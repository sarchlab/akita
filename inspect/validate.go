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
	// Spec fields must be scalars or slices of scalars even when the Spec
	// customizes its JSON.
	hasUnexported, encodesField := false, false
	for i := range st.NumFields() {
		f := st.Field(i)
		if err := validateSpecFieldType(f.Type()); err != nil {
			return posErrorf(pkg, f.Pos(), "Spec field %s: %v", f.Name(), err)
		}
		if !f.Exported() {
			hasUnexported = true
		} else if encodedWhenZero(reflect.StructTag(st.Tag(i))) {
			encodesField = true
		}
	}
	if jsonPkg := index["encoding/json"]; jsonPkg != nil {
		marshaler := jsonPkg.Types.Scope().Lookup("Marshaler").Type().Underlying().(*types.Interface)
		unmarshaler := jsonPkg.Types.Scope().Lookup("Unmarshaler").Type().Underlying().(*types.Interface)
		if types.Implements(typ, marshaler) {
			if !types.Implements(types.NewPointer(typ), unmarshaler) {
				return fmt.Errorf("%s: Spec customizes MarshalJSON but has no UnmarshalJSON", pkg.PkgPath)
			}
			return nil
		}
	}
	// Mirror modeling's data-loss guard: a Spec whose state is unexported
	// serializes as {} and would lose it across a checkpoint.
	if hasUnexported && !encodesField {
		return fmt.Errorf("%s: Spec has unexported fields but serializes as {}; "+
			"export the fields", pkg.PkgPath)
	}
	return nil
}

// encodedWhenZero reports whether encoding/json writes an exported field whose
// value is zero: it is not tagged "-" and has neither the omitempty nor the
// omitzero option.
func encodedWhenZero(tag reflect.StructTag) bool {
	jsonTag := tag.Get("json")
	if jsonTag == "-" {
		return false
	}

	_, opts, _ := strings.Cut(jsonTag, ",")
	options := strings.Split(opts, ",")

	return !slices.Contains(options, "omitempty") &&
		!slices.Contains(options, "omitzero")
}

// validateSpecFieldType accepts scalar field types (booleans, integers except
// uintptr, floats, strings, and named types based on them) and slices and
// arrays of scalars.
func validateSpecFieldType(typ types.Type) error {
	if isScalarType(typ) {
		return nil
	}

	switch t := typ.Underlying().(type) {
	case *types.Slice:
		if isScalarType(t.Elem()) {
			return nil
		}
	case *types.Array:
		if isScalarType(t.Elem()) {
			return nil
		}
	}

	return fmt.Errorf("disallowed Spec type %s: Spec fields must be scalars "+
		"or slices of scalars", typ)
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
