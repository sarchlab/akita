package inspect

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

// definitionVarName is the required name of the package-level definition var.
const definitionVarName = "Definition"

// extractors maps the fully-qualified name of a definition type to the
// extractor for its definition kind. A package-level var of such a type is a
// definition. Adding a kind (e.g. a benchmark definition) means adding an
// entry here plus its extractor.
var extractors = map[string]func(
	pkg *packages.Package, lit *ast.CompositeLit, index pkgIndex,
) (*schema.Definition, error){
	"github.com/sarchlab/akita/v5/modeling/ticking.Definition": extractModel(
		schema.ModelTicking),
	"github.com/sarchlab/akita/v5/modeling/wakeup.Definition": extractModel(
		schema.ModelWakeup),
	"github.com/sarchlab/akita/v5/modeling/event.Definition": extractModel(
		schema.ModelEvent),
}

// extractPackage finds the definition in pkg, if any, and extracts it. The
// bool reports whether the package has a definition.
func extractPackage(
	pkg *packages.Package, index pkgIndex,
) (schema.Definition, bool, error) {
	var found schema.Definition
	hasDef := false

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}

			for _, spec := range gen.Specs {
				def, ok, err := extractVarSpec(
					pkg, spec.(*ast.ValueSpec), index)
				if err != nil {
					return schema.Definition{}, false, err
				}
				if !ok {
					continue
				}

				if hasDef {
					return schema.Definition{}, false, fmt.Errorf(
						"%s: more than one definition in package",
						pkg.PkgPath)
				}
				found, hasDef = def, true
			}
		}
	}

	return found, hasDef, nil
}

// extractVarSpec checks whether the value spec declares a var of a
// definition type and, if so, extracts the definition from its literal
// initializer. The bool reports whether the spec declares one.
func extractVarSpec(
	pkg *packages.Package, vs *ast.ValueSpec, index pkgIndex,
) (schema.Definition, bool, error) {
	for i, name := range vs.Names {
		obj := pkg.TypesInfo.Defs[name]
		if obj == nil {
			continue
		}

		typ := types.Unalias(obj.Type())
		if ptr, ok := typ.(*types.Pointer); ok {
			if kind := definitionTypeName(ptr.Elem()); extractors[kind] != nil {
				return schema.Definition{}, false, posErrorf(pkg, name.Pos(),
					"%s must be a %s value, not a pointer", name.Name, kind)
			}
		}

		extractor := extractors[definitionTypeName(typ)]
		if extractor == nil {
			continue
		}

		if name.Name != definitionVarName {
			return schema.Definition{}, false, posErrorf(pkg, name.Pos(),
				"definition var must be named %q, found %q",
				definitionVarName, name.Name)
		}

		var lit *ast.CompositeLit
		if i < len(vs.Values) {
			lit, _ = vs.Values[i].(*ast.CompositeLit)
		}
		if lit == nil {
			return schema.Definition{}, false, posErrorf(pkg, name.Pos(),
				"%s must be initialized with a composite literal, "+
					"not a value computed elsewhere", definitionVarName)
		}

		def, err := extractor(pkg, lit, index)
		if err != nil {
			return schema.Definition{}, false, err
		}

		def.SchemaVersion = schema.Version
		def.Package = pkg.PkgPath
		if pkg.Module != nil {
			def.Module = pkg.Module.Path
		}

		return *def, true, nil
	}

	return schema.Definition{}, false, nil
}

// definitionTypeName returns "importpath.TypeName" of a named type's generic
// origin (so ticking.Definition[S, T, R, P, M] maps to ticking.Definition),
// or "" for other types. Aliases, including generic ones, resolve to the type
// they denote.
func definitionTypeName(typ types.Type) string {
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return ""
	}

	return qualifiedTypeName(named.Origin())
}

// keyedElements returns the keyed fields of a composite literal, requiring
// every element to be keyed.
func keyedElements(
	pkg *packages.Package, lit *ast.CompositeLit,
) (map[string]ast.Expr, error) {
	out := map[string]ast.Expr{}

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return nil, posErrorf(pkg, elt.Pos(),
				"literal fields must be keyed (Field: value)")
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return nil, posErrorf(pkg, kv.Key.Pos(),
				"literal keys must be field names")
		}

		out[key.Name] = kv.Value
	}

	return out, nil
}

// evalStructLiteral evaluates a struct composite literal with constant
// leaves into a map from field name to Go value.
func evalStructLiteral(
	pkg *packages.Package, expr ast.Expr,
) (map[string]any, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, posErrorf(pkg, expr.Pos(),
			"DefaultSpec must be a struct literal, "+
				"not a value computed elsewhere")
	}

	fields, err := keyedElements(pkg, lit)
	if err != nil {
		return nil, err
	}

	out := map[string]any{}
	for name, value := range fields {
		v, err := evalConstExpr(pkg, value)
		if err != nil {
			return nil, err
		}
		out[name] = v
	}

	return out, nil
}

// evalConstExpr evaluates an expression that must be statically evaluable: a
// constant expression.
func evalConstExpr(pkg *packages.Package, expr ast.Expr) (any, error) {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return evalConstExpr(pkg, paren.X)
	}

	tv, ok := pkg.TypesInfo.Types[expr]
	if !ok || tv.Value == nil {
		return nil, posErrorf(pkg, expr.Pos(),
			"not statically analyzable: value is not a constant expression")
	}

	return constantValue(pkg, expr, tv)
}

// zeroValue returns the default of a field the DefaultSpec literal leaves
// out, in the same representation as constantValue.
func zeroValue(typ types.Type) any {
	t, ok := typ.Underlying().(*types.Basic)
	if !ok {
		return nil
	}

	switch {
	case t.Info()&types.IsBoolean != 0:
		return false
	case t.Info()&types.IsUnsigned != 0:
		return uint64(0)
	case t.Info()&types.IsInteger != 0:
		return int64(0)
	case t.Info()&types.IsFloat != 0:
		return float64(0)
	case t.Info()&types.IsString != 0:
		return ""
	default:
		return nil
	}
}

// constantValue converts a folded constant to a plain Go value based on the
// expression's type.
func constantValue(
	pkg *packages.Package, expr ast.Expr, tv types.TypeAndValue,
) (any, error) {
	basic, ok := tv.Type.Underlying().(*types.Basic)
	if !ok {
		return nil, posErrorf(pkg, expr.Pos(),
			"unsupported constant type %s", tv.Type)
	}

	switch {
	case basic.Info()&types.IsBoolean != 0:
		return constant.BoolVal(tv.Value), nil
	case basic.Info()&types.IsUnsigned != 0:
		v, ok := constant.Uint64Val(tv.Value)
		if !ok {
			return nil, posErrorf(pkg, expr.Pos(), "constant overflows uint64")
		}
		return v, nil
	case basic.Info()&types.IsInteger != 0:
		v, ok := constant.Int64Val(tv.Value)
		if !ok {
			return nil, posErrorf(pkg, expr.Pos(), "constant overflows int64")
		}
		return v, nil
	case basic.Info()&types.IsFloat != 0:
		v, _ := constant.Float64Val(tv.Value)
		return v, nil
	case basic.Info()&types.IsString != 0:
		return constant.StringVal(tv.Value), nil
	default:
		return nil, posErrorf(pkg, expr.Pos(),
			"unsupported constant type %s", tv.Type)
	}
}

func constString(pkg *packages.Package, expr ast.Expr) (string, error) {
	v, err := evalConstExpr(pkg, expr)
	if err != nil {
		return "", err
	}

	s, ok := v.(string)
	if !ok {
		return "", posErrorf(pkg, expr.Pos(), "expected a string constant")
	}

	return s, nil
}

func posErrorf(
	pkg *packages.Package, pos token.Pos, format string, args ...any,
) error {
	return fmt.Errorf("%s: %s",
		pkg.Fset.Position(pos), fmt.Sprintf(format, args...))
}
