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

// builderTypeName and withResourcesMethod locate a component's Resources
// type: the parameter of its builder's WithResources method.
const (
	builderTypeName     = "Builder"
	withResourcesMethod = "WithResources"
)

// extractors maps the fully-qualified name of a definition type to the
// extractor for its definition kind. A package-level var of such a type is a
// definition. Adding a kind (e.g. a benchmark definition) means adding an
// entry here plus its extractor.
var extractors = map[string]func(
	pkg *packages.Package, lit *ast.CompositeLit, index pkgIndex,
) (*schema.Definition, error){
	"github.com/sarchlab/akita/v5/modeling.ComponentDef":       extractComponent,
	"github.com/sarchlab/akita/v5/modeling/ticking.Definition": extractTicking,
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
// origin (so ComponentDef[Spec] maps to ComponentDef), or "" for other types.
// Aliases, including generic ones, resolve to the type they denote.
func definitionTypeName(typ types.Type) string {
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return ""
	}

	return qualifiedTypeName(named.Origin())
}

// extractComponent extracts a ComponentDef composite literal with
// statically evaluable leaves.
func extractComponent(
	pkg *packages.Package, lit *ast.CompositeLit, index pkgIndex,
) (*schema.Definition, error) {
	specType, err := componentSpecType(pkg, lit)
	if err != nil {
		return nil, err
	}

	if err := validateSpecType(pkg, specType, index); err != nil {
		return nil, err
	}

	def := &schema.Definition{Kind: schema.KindComponent}

	defaults, err := applyComponentFields(pkg, lit, def, index)
	if err != nil {
		return nil, err
	}

	def.Spec, err = structFields(pkg, specType, defaults, index)
	if err != nil {
		return nil, err
	}

	resType, err := builderResourcesType(pkg)
	if err != nil {
		return nil, err
	}

	if resType != nil {
		def.Resources, err = structFields(pkg, resType, nil, index)
		if err != nil {
			return nil, err
		}
	}

	if err := validateDefinition(pkg, lit, specType, def); err != nil {
		return nil, err
	}

	return def, nil
}

// applyComponentFields walks the keyed fields of the ComponentDef literal
// into def and returns the evaluated DefaultSpec values.
func applyComponentFields(
	pkg *packages.Package, lit *ast.CompositeLit,
	def *schema.Definition, index pkgIndex,
) (map[string]any, error) {
	fields, err := keyedElements(pkg, lit)
	if err != nil {
		return nil, err
	}

	defaults := map[string]any{}

	for key, value := range fields {
		switch key {
		case "Name":
			def.Name, err = constString(pkg, value)
		case "DefaultSpec":
			defaults, err = evalStructLiteral(pkg, value)
		case "Ports":
			def.Ports, err = extractPorts(pkg, value, index)
		default:
			err = posErrorf(pkg, value.Pos(),
				"unsupported ComponentDef field %q", key)
		}
		if err != nil {
			return nil, err
		}
	}

	return defaults, nil
}

// componentSpecType returns the Spec type argument of the ComponentDef
// literal.
func componentSpecType(
	pkg *packages.Package, lit *ast.CompositeLit,
) (types.Type, error) {
	tv, ok := pkg.TypesInfo.Types[lit]
	if !ok {
		return nil, posErrorf(pkg, lit.Pos(),
			"cannot resolve ComponentDef literal type")
	}

	named, ok := types.Unalias(tv.Type).(*types.Named)
	if !ok || named.TypeArgs().Len() != 1 {
		return nil, posErrorf(pkg, lit.Pos(),
			"ComponentDef literal must be instantiated with [Spec]")
	}

	return named.TypeArgs().At(0), nil
}

// builderResourcesType returns the Resources struct taken by the package's
// Builder.WithResources method: the external references a caller supplies at
// construction. A pointer or variadic parameter resolves to its struct. It
// returns nil when the package has no Builder or the Builder takes no
// resources, and an error when WithResources does not take one struct.
func builderResourcesType(pkg *packages.Package) (types.Type, error) {
	obj, ok := pkg.Types.Scope().Lookup(builderTypeName).(*types.TypeName)
	if !ok {
		return nil, nil //nolint:nilnil // No Builder means no resources.
	}

	// The pointer method set holds both value- and pointer-receiver methods.
	sel := types.NewMethodSet(types.NewPointer(obj.Type())).
		Lookup(pkg.Types, withResourcesMethod)
	if sel == nil {
		return nil, nil //nolint:nilnil // A Builder may take no resources.
	}

	sig, ok := sel.Obj().Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 {
		return nil, posErrorf(pkg, sel.Obj().Pos(),
			"%s.%s must take a single Resources struct",
			builderTypeName, withResourcesMethod)
	}

	param := sig.Params().At(0).Type()
	if sig.Variadic() {
		param = param.(*types.Slice).Elem()
	}
	if ptr, ok := types.Unalias(param).(*types.Pointer); ok {
		param = ptr.Elem()
	}

	if _, ok := param.Underlying().(*types.Struct); !ok {
		return nil, posErrorf(pkg, sel.Obj().Pos(),
			"%s.%s must take a Resources struct, got %s",
			builderTypeName, withResourcesMethod, sig.Params().At(0).Type())
	}

	return param, nil
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

// extractPorts extracts a []PortDef literal.
func extractPorts(
	pkg *packages.Package, expr ast.Expr, index pkgIndex,
) ([]schema.Port, error) {
	if tv, ok := pkg.TypesInfo.Types[expr]; ok && tv.IsNil() {
		return nil, nil //nolint:nilnil // Ports: nil declares no ports.
	}

	elems, err := sliceElements(pkg, expr)
	if err != nil {
		return nil, err
	}

	ports := make([]schema.Port, 0, len(elems))
	for _, elem := range elems {
		fields, err := keyedElements(pkg, elem)
		if err != nil {
			return nil, err
		}

		var port schema.Port
		for key, value := range fields {
			switch key {
			case "Name":
				port.Name, err = constString(pkg, value)
			case "Roles":
				port.Roles, err = extractRoles(pkg, value, index)
			case "Group":
				port.Group, err = constBool(pkg, value)
			default:
				err = posErrorf(pkg, value.Pos(),
					"unsupported PortDef field %q", key)
			}
			if err != nil {
				return nil, err
			}
		}

		ports = append(ports, port)
	}

	return ports, nil
}

// sliceElements returns the elements of a slice composite literal, each of
// which must itself be a composite literal.
func sliceElements(
	pkg *packages.Package, expr ast.Expr,
) ([]*ast.CompositeLit, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, posErrorf(pkg, expr.Pos(),
			"expected a slice literal, not a value computed elsewhere")
	}

	elems := make([]*ast.CompositeLit, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		el, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, posErrorf(pkg, elt.Pos(),
				"slice elements must be literals")
		}
		elems = append(elems, el)
	}

	return elems, nil
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

func constBool(pkg *packages.Package, expr ast.Expr) (bool, error) {
	v, err := evalConstExpr(pkg, expr)
	if err != nil {
		return false, err
	}

	b, ok := v.(bool)
	if !ok {
		return false, posErrorf(pkg, expr.Pos(), "expected a boolean constant")
	}

	return b, nil
}

func posErrorf(
	pkg *packages.Package, pos token.Pos, format string, args ...any,
) error {
	return fmt.Errorf("%s: %s",
		pkg.Fset.Position(pos), fmt.Sprintf(format, args...))
}
