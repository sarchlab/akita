package inspect

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"golang.org/x/tools/go/packages"
)

// componentModels identifies the three supported component types.
var componentModels = map[string]string{
	"github.com/sarchlab/akita/v5/sim/modeling/ticking.Component": schema.ModelTicking,
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup.Component":  schema.ModelWakeup,
	"github.com/sarchlab/akita/v5/sim/modeling/event.Component":   schema.ModelEvent,
}

// extractPackage discovers a component through its Comp type alias and reads
// DefaultSpec without executing any package code.
func extractPackage(pkg *packages.Package, index pkgIndex) (schema.Definition, bool, error) {
	obj := pkg.Types.Scope().Lookup("Comp")
	if obj == nil {
		return extractDomainPackage(pkg, index)
	}
	typ := types.Unalias(obj.Type())
	if pointer, ok := typ.(*types.Pointer); ok {
		if componentModels[componentTypeName(pointer.Elem())] != "" {
			return schema.Definition{}, false, posErrorf(pkg, obj.Pos(),
				"Comp must alias a component value type, not a pointer")
		}
	}
	model := componentModels[componentTypeName(typ)]
	if model == "" {
		if defaults := pkg.Types.Scope().Lookup("DefaultSpec"); defaults != nil {
			return schema.Definition{}, false, posErrorf(pkg, obj.Pos(),
				"Comp must alias an event, ticking, or wakeup Component type")
		}
		return schema.Definition{}, false, nil
	}
	declaration, ok := obj.(*types.TypeName)
	if !ok || !declaration.IsAlias() {
		return schema.Definition{}, false, posErrorf(pkg, obj.Pos(), "Comp must be a type alias")
	}
	if alias, ok := obj.Type().(*types.Alias); ok && alias.TypeParams().Len() != 0 {
		return schema.Definition{}, false, posErrorf(pkg, obj.Pos(), "Comp must have five concrete element types")
	}
	component := typ.(*types.Named)
	if component.TypeArgs().Len() != 5 {
		return schema.Definition{}, false, posErrorf(pkg, obj.Pos(), "Comp must have five concrete element types")
	}
	def, err := extractModelDefinition(pkg, component, index, model)
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

// componentTypeName resolves aliases, including generic aliases, to their
// named origin (for example ticking.Component).
func componentTypeName(typ types.Type) string {
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return ""
	}
	return qualifiedTypeName(named.Origin())
}

// defaultSpecLiteral accepts exactly func DefaultSpec() Spec { return Spec{...} }.
// It does not call the function or interpret arbitrary Go statements.
func defaultSpecLiteral(pkg *packages.Package, specType types.Type, anchor token.Pos) (*ast.CompositeLit, error) {
	obj := pkg.Types.Scope().Lookup("DefaultSpec")
	if obj == nil {
		return nil, posErrorf(pkg, anchor, "definition requires func DefaultSpec() Spec")
	}
	function, ok := obj.(*types.Func)
	if !ok {
		return nil, posErrorf(pkg, obj.Pos(), "DefaultSpec must be a package-level function")
	}
	sig := function.Type().(*types.Signature)
	if sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Params().Len() != 0 ||
		sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), specType) {
		return nil, posErrorf(pkg, obj.Pos(), "DefaultSpec must have signature func DefaultSpec() Spec")
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || pkg.TypesInfo.Defs[fn.Name] != obj {
				continue
			}
			if fn.Body != nil && len(fn.Body.List) == 1 {
				if ret, ok := fn.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
					if lit, ok := ast.Unparen(ret.Results[0]).(*ast.CompositeLit); ok {
						return lit, nil
					}
				}
			}
			return nil, posErrorf(pkg, fn.Pos(), "DefaultSpec must contain only return Spec{...}")
		}
	}
	return nil, posErrorf(pkg, obj.Pos(), "cannot read DefaultSpec function body")
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
		v, err := evalSpecValue(pkg, value)
		if err != nil {
			return nil, err
		}
		out[name] = v
	}

	return out, nil
}

// evalSpecValue evaluates a field value of a DefaultSpec literal: a constant
// expression, or, for a slice or array field, nil or a literal whose elements
// are constant expressions. A slice or array becomes a []any.
func evalSpecValue(pkg *packages.Package, expr ast.Expr) (any, error) {
	expr = ast.Unparen(expr)

	tv := pkg.TypesInfo.Types[expr]
	if tv.IsNil() {
		return []any{}, nil
	}

	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return evalConstExpr(pkg, expr)
	}

	var elems []any
	switch t := tv.Type.Underlying().(type) {
	case *types.Slice:
		elems = make([]any, 0, len(lit.Elts))
	case *types.Array:
		elems = make([]any, 0, t.Len())
	case *types.Struct:
		explicit, err := evalStructLiteral(pkg, lit)
		if err != nil {
			return nil, err
		}
		record := zeroValue(tv.Type).(map[string]any)
		for name, value := range explicit {
			record[name] = value
		}
		return record, nil
	default:
		return evalConstExpr(pkg, expr)
	}

	for _, elt := range lit.Elts {
		if _, keyed := elt.(*ast.KeyValueExpr); keyed {
			return nil, posErrorf(pkg, elt.Pos(),
				"slice literal elements must not be keyed")
		}

		v, err := evalSpecValue(pkg, elt)
		if err != nil {
			return nil, err
		}

		elems = append(elems, v)
	}

	if arr, ok := tv.Type.Underlying().(*types.Array); ok {
		for int64(len(elems)) < arr.Len() {
			elems = append(elems, zeroValue(arr.Elem()))
		}
	}

	return elems, nil
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
	switch u := typ.Underlying().(type) {
	case *types.Struct:
		record := map[string]any{}
		for i := range u.NumFields() {
			record[u.Field(i).Name()] = zeroValue(u.Field(i).Type())
		}
		return record
	case *types.Slice:
		return []any{}
	case *types.Array:
		elems := make([]any, u.Len())
		for i := range elems {
			elems[i] = zeroValue(u.Elem())
		}

		return elems
	}

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
