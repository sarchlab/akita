package inspect

import (
	"fmt"
	"go/ast"
	"go/types"
	"reflect"

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
	for i := range st.NumFields() {
		f := st.Field(i)
		if reflect.StructTag(st.Tag(i)).Get("json") == "-" {
			continue
		}
		if err := validateSpecFieldType(f.Type()); err != nil {
			return posErrorf(pkg, f.Pos(), "Spec field %s: %v", f.Name(), err)
		}
	}
	return nil
}

// validateSpecFieldType accepts only scalar field types: booleans, integers
// (except uintptr), floats, strings, and named types based on them.
func validateSpecFieldType(typ types.Type) error {
	if t, ok := typ.Underlying().(*types.Basic); ok && t.Kind() != types.Uintptr &&
		t.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) != 0 {
		return nil
	}

	return fmt.Errorf("disallowed Spec type %s: Spec fields must be scalars", typ)
}

// validateDefinition checks the definition's metadata invariants. The
// runtime does not re-check them, so the inspector is where they are enforced.
func validateDefinition(
	pkg *packages.Package, lit *ast.CompositeLit,
	specType types.Type, def *schema.Definition,
) error {
	if def.Name == "" {
		return posErrorf(pkg, lit.Pos(), "component definition must have a name")
	}
	seen := map[string]bool{}
	checkName := func(name string) error {
		if name == "" {
			return posErrorf(pkg, lit.Pos(), "port has an empty name")
		}
		if seen[name] {
			return posErrorf(pkg, lit.Pos(), "port %q declared more than once", name)
		}
		seen[name] = true
		return nil
	}
	for _, p := range def.Ports {
		if err := checkName(p.Name); err != nil {
			return err
		}
	}
	return validateFieldMetadata(pkg, specType, def.Spec)
}

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
