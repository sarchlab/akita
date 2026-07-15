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

// extractors maps the fully-qualified name of a Define* function to the
// extractor for its definition kind. Adding a kind (e.g. DefineBenchmark)
// means adding an entry here plus its extractor.
var extractors = map[string]func(
	pkg *packages.Package, call *ast.CallExpr, index pkgIndex,
) (*schema.Definition, error){
	"github.com/sarchlab/akita/v5/modeling.DefineComponent": extractComponent,
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

// extractVarSpec checks whether the value spec initializes a var with a
// Define* call and, if so, extracts the definition. The bool reports whether
// the spec declares one.
func extractVarSpec(
	pkg *packages.Package, vs *ast.ValueSpec, index pkgIndex,
) (schema.Definition, bool, error) {
	for i, value := range vs.Values {
		call, ok := value.(*ast.CallExpr)
		if !ok {
			continue
		}

		extractor := extractorFor(pkg, call)
		if extractor == nil {
			continue
		}

		if vs.Names[i].Name != definitionVarName {
			return schema.Definition{}, false, posErrorf(pkg,
				vs.Names[i].Pos(),
				"definition var must be named %q, found %q",
				definitionVarName, vs.Names[i].Name)
		}

		def, err := extractor(pkg, call, index)
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

// extractorFor resolves the call's callee through type information and looks
// it up in the extractor registry. It returns nil if the call is not a
// Define* call.
func extractorFor(pkg *packages.Package, call *ast.CallExpr) func(
	*packages.Package, *ast.CallExpr, pkgIndex,
) (*schema.Definition, error) {
	fun := call.Fun

	// Strip explicit type instantiation: DefineComponent[S, R](...).
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}

	var ident *ast.Ident
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		ident = f.Sel
	case *ast.Ident:
		ident = f
	default:
		return nil
	}

	fn, ok := pkg.TypesInfo.Uses[ident].(*types.Func)
	if !ok {
		return nil
	}

	return extractors[fn.FullName()]
}

// extractComponent extracts a DefineComponent call: one ComponentDef
// composite literal with statically evaluable leaves.
func extractComponent(
	pkg *packages.Package, call *ast.CallExpr, index pkgIndex,
) (*schema.Definition, error) {
	if len(call.Args) != 1 {
		return nil, posErrorf(pkg, call.Pos(),
			"DefineComponent must be called with a single ComponentDef literal")
	}

	lit, ok := call.Args[0].(*ast.CompositeLit)
	if !ok {
		return nil, posErrorf(pkg, call.Args[0].Pos(),
			"DefineComponent argument must be a composite literal, "+
				"not a value computed elsewhere")
	}

	specType, resType, err := componentTypeArgs(pkg, lit)
	if err != nil {
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

	def.Resources, err = structFields(pkg, resType, nil, index)
	if err != nil {
		return nil, err
	}

	if err := checkCountFields(pkg, lit, specType, def); err != nil {
		return nil, err
	}

	return def, nil
}

// checkCountFields validates every port group's CountField against the Spec,
// mirroring the runtime checks in modeling.DefineComponent: the field must
// exist (by JSON name), be an integer, and be tagged derived. The runtime
// panics on violations, but a definition that is never executed would
// otherwise slip through the inspector.
func checkCountFields(
	pkg *packages.Package, lit *ast.CompositeLit,
	specType types.Type, def *schema.Definition,
) error {
	specByJSON := map[string]schema.Field{}
	for _, f := range def.Spec {
		if f.JSONName != "" {
			specByJSON[f.JSONName] = f
		}
	}

	for _, g := range def.PortGroups {
		if g.CountField == "" {
			continue
		}

		f, ok := specByJSON[g.CountField]
		if !ok {
			return posErrorf(pkg, lit.Pos(),
				"port group %q: CountField %q does not match any Spec "+
					"field's JSON name", g.Name, g.CountField)
		}

		if !isIntegerField(specType, f.Name) {
			return posErrorf(pkg, lit.Pos(),
				"port group %q: CountField %q must be an integer Spec field",
				g.Name, g.CountField)
		}

		if !f.Derived {
			return posErrorf(pkg, lit.Pos(),
				"port group %q: CountField %q must be tagged "+
					"`akita:\"derived\"`", g.Name, g.CountField)
		}
	}

	return nil
}

// isIntegerField reports whether the named field of the struct type has an
// integer underlying type.
func isIntegerField(specType types.Type, fieldName string) bool {
	st, ok := specType.Underlying().(*types.Struct)
	if !ok {
		return false
	}

	for f := range st.Fields() {
		if f.Name() != fieldName {
			continue
		}

		basic, ok := f.Type().Underlying().(*types.Basic)
		return ok && basic.Info()&types.IsInteger != 0
	}

	return false
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

	var defaults map[string]any

	for key, value := range fields {
		switch key {
		case "Name":
			def.Name, err = constString(pkg, value)
		case "DefaultSpec":
			defaults, err = evalStructLiteral(pkg, value)
		case "Ports":
			def.Ports, err = extractPorts(pkg, value, index)
		case "PortGroups":
			def.PortGroups, err = extractPortGroups(pkg, value, index)
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

// componentTypeArgs returns the Spec and Resources type arguments of the
// ComponentDef literal.
func componentTypeArgs(
	pkg *packages.Package, lit *ast.CompositeLit,
) (spec, res types.Type, err error) {
	tv, ok := pkg.TypesInfo.Types[lit]
	if !ok {
		return nil, nil, posErrorf(pkg, lit.Pos(),
			"cannot resolve ComponentDef literal type")
	}

	named, ok := tv.Type.(*types.Named)
	if !ok || named.TypeArgs().Len() != 2 {
		return nil, nil, posErrorf(pkg, lit.Pos(),
			"ComponentDef literal must be instantiated with [Spec, Resources]")
	}

	return named.TypeArgs().At(0), named.TypeArgs().At(1), nil
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

// extractPortGroups extracts a []PortGroupDef literal.
func extractPortGroups(
	pkg *packages.Package, expr ast.Expr, index pkgIndex,
) ([]schema.PortGroup, error) {
	elems, err := sliceElements(pkg, expr)
	if err != nil {
		return nil, err
	}

	groups := make([]schema.PortGroup, 0, len(elems))
	for _, elem := range elems {
		fields, err := keyedElements(pkg, elem)
		if err != nil {
			return nil, err
		}

		var group schema.PortGroup
		for key, value := range fields {
			switch key {
			case "Name":
				group.Name, err = constString(pkg, value)
			case "Roles":
				group.Roles, err = extractRoles(pkg, value, index)
			case "MinCount":
				group.MinCount, err = constInt(pkg, value)
			case "MaxCount":
				group.MaxCount, err = constInt(pkg, value)
			case "CountField":
				group.CountField, err = constString(pkg, value)
			default:
				err = posErrorf(pkg, value.Pos(),
					"unsupported PortGroupDef field %q", key)
			}
			if err != nil {
				return nil, err
			}
		}

		groups = append(groups, group)
	}

	return groups, nil
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
// constant expression, or a slice/map composite literal of such expressions.
func evalConstExpr(pkg *packages.Package, expr ast.Expr) (any, error) {
	if lit, ok := expr.(*ast.CompositeLit); ok {
		return evalCompositeConst(pkg, lit)
	}

	tv, ok := pkg.TypesInfo.Types[expr]
	if !ok || tv.Value == nil {
		return nil, posErrorf(pkg, expr.Pos(),
			"not statically analyzable: value is not a constant expression")
	}

	return constantValue(pkg, expr, tv)
}

func evalCompositeConst(
	pkg *packages.Package, lit *ast.CompositeLit,
) (any, error) {
	tv, ok := pkg.TypesInfo.Types[lit]
	if !ok {
		return nil, posErrorf(pkg, lit.Pos(), "cannot resolve literal type")
	}

	switch tv.Type.Underlying().(type) {
	case *types.Slice:
		out := make([]any, 0, len(lit.Elts))
		for _, elt := range lit.Elts {
			v, err := evalConstExpr(pkg, elt)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil

	case *types.Map:
		out := map[string]any{}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				return nil, posErrorf(pkg, elt.Pos(),
					"map literal entries must be key: value")
			}
			k, err := evalConstExpr(pkg, kv.Key)
			if err != nil {
				return nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, posErrorf(pkg, kv.Key.Pos(),
					"map keys must be strings")
			}
			v, err := evalConstExpr(pkg, kv.Value)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil

	default:
		return nil, posErrorf(pkg, lit.Pos(),
			"not statically analyzable: nested %s literal", tv.Type)
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

func constInt(pkg *packages.Package, expr ast.Expr) (int, error) {
	v, err := evalConstExpr(pkg, expr)
	if err != nil {
		return 0, err
	}

	n, ok := v.(int64)
	if !ok {
		return 0, posErrorf(pkg, expr.Pos(), "expected an integer constant")
	}

	return int(n), nil
}

func posErrorf(
	pkg *packages.Package, pos token.Pos, format string, args ...any,
) error {
	return fmt.Errorf("%s: %s",
		pkg.Fset.Position(pos), fmt.Sprintf(format, args...))
}
