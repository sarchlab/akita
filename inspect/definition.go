package inspect

import (
	"go/ast"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

const (
	messagingPath  = "github.com/sarchlab/akita/v5/messaging"
	modelingPath   = "github.com/sarchlab/akita/v5/modeling"
	portTypeName   = messagingPath + ".Port"
	middlewareName = "Middleware"
)

// extractModel returns the extractor for the Definition literal of a
// component model (modeling/ticking, modeling/wakeup, or modeling/event): a
// component defined by five structs. The component type is identified by its
// package, so its name is the package name. The Spec defaults come from the
// literal; the Resources, Ports, and Middlewares come from the type
// arguments.
func extractModel(model string) func(
	pkg *packages.Package, lit *ast.CompositeLit, index pkgIndex,
) (*schema.Definition, error) {
	return func(
		pkg *packages.Package, lit *ast.CompositeLit, index pkgIndex,
	) (*schema.Definition, error) {
		return extractModelDefinition(pkg, lit, index, model)
	}
}

func extractModelDefinition(
	pkg *packages.Package, lit *ast.CompositeLit, index pkgIndex, model string,
) (*schema.Definition, error) {
	args, err := definitionTypeArgs(pkg, lit)
	if err != nil {
		return nil, err
	}
	specType, resType, portsType, mwType := args[0], args[2], args[3], args[4]

	if err := validateSpecType(pkg, specType, index); err != nil {
		return nil, err
	}

	def := &schema.Definition{
		Kind:  schema.KindComponent,
		Model: model,
		Name:  pkg.Types.Name(),
	}

	defaults, err := applyDefinitionFields(pkg, lit)
	if err != nil {
		return nil, err
	}

	if def.Spec, err = structFields(pkg, specType, defaults, index); err != nil {
		return nil, err
	}

	if err := validateFieldMetadata(pkg, specType, def.Spec); err != nil {
		return nil, err
	}

	if def.Resources, err = resourcesOfType(pkg, lit, resType, index); err != nil {
		return nil, err
	}

	if def.Ports, err = portsOfType(pkg, lit, portsType, index); err != nil {
		return nil, err
	}

	if def.Middlewares, err = middlewaresOfType(pkg, lit, mwType, index); err != nil {
		return nil, err
	}

	return def, nil
}

// definitionTypeArgs returns the five type arguments of the Definition
// literal: Spec, State, Resources, Ports, and Middlewares.
func definitionTypeArgs(
	pkg *packages.Package, lit *ast.CompositeLit,
) ([]types.Type, error) {
	tv, ok := pkg.TypesInfo.Types[lit]
	if !ok {
		return nil, posErrorf(pkg, lit.Pos(), "cannot resolve Definition literal type")
	}

	named, ok := types.Unalias(tv.Type).(*types.Named)
	if !ok || named.TypeArgs().Len() != 5 {
		return nil, posErrorf(pkg, lit.Pos(),
			"Definition literal must be instantiated with "+
				"[Spec, State, Resources, Ports, Middlewares]")
	}

	args := make([]types.Type, 5)
	for i := range args {
		args[i] = named.TypeArgs().At(i)
	}

	return args, nil
}

// applyDefinitionFields walks the keyed fields of the Definition literal and
// returns the evaluated DefaultSpec values. NewState and NewMiddlewares must
// name functions.
func applyDefinitionFields(
	pkg *packages.Package, lit *ast.CompositeLit,
) (map[string]any, error) {
	fields, err := keyedElements(pkg, lit)
	if err != nil {
		return nil, err
	}

	defaults := map[string]any{}
	for key, value := range fields {
		switch key {
		case "DefaultSpec":
			defaults, err = evalStructLiteral(pkg, value)
		case "NewState", "NewMiddlewares":
			err = functionRef(pkg, key, value)
		default:
			err = posErrorf(pkg, value.Pos(), "unsupported Definition field %q", key)
		}
		if err != nil {
			return nil, err
		}
	}

	if _, ok := fields["NewMiddlewares"]; !ok {
		return nil, posErrorf(pkg, lit.Pos(), "Definition must set NewMiddlewares")
	}

	return defaults, nil
}

// functionRef checks that a Definition function field names a function.
func functionRef(pkg *packages.Package, field string, expr ast.Expr) error {
	obj, err := identObject(pkg, expr)
	if err != nil {
		return posErrorf(pkg, expr.Pos(), "%s must name a function", field)
	}

	if _, ok := obj.(*types.Func); !ok {
		return posErrorf(pkg, expr.Pos(), "%s must name a function, not %s",
			field, obj.Name())
	}

	return nil
}

// identObject resolves an identifier or selector expression to the
// package-level object it references.
func identObject(pkg *packages.Package, expr ast.Expr) (types.Object, error) {
	var ident *ast.Ident
	switch e := expr.(type) {
	case *ast.Ident:
		ident = e
	case *ast.SelectorExpr:
		ident = e.Sel
	default:
		return nil, posErrorf(pkg, expr.Pos(),
			"expected an identifier referencing a package-level declaration")
	}

	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil || obj.Pkg() == nil {
		return nil, posErrorf(pkg, expr.Pos(),
			"cannot resolve identifier %s", ident.Name)
	}

	return obj, nil
}

// resourcesOfType describes the fields of a Resources struct: the references
// to shared objects that the system builder supplies.
func resourcesOfType(
	pkg *packages.Package, lit *ast.CompositeLit, typ types.Type, index pkgIndex,
) ([]schema.Field, error) {
	if _, ok := typ.Underlying().(*types.Struct); !ok {
		return nil, posErrorf(pkg, lit.Pos(), "Resources type %s must be a struct", typ)
	}

	return structFields(pkg, typ, nil, index)
}

// portsOfType describes the fields of a Ports struct: a messaging.Port field
// is a port and a []messaging.Port field a port group. Roles come from
// `akita:"role=<protocol>/<role>"` tags and must name a defined protocol role.
func portsOfType(
	pkg *packages.Package, lit *ast.CompositeLit, typ types.Type, index pkgIndex,
) ([]schema.Port, error) {
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, posErrorf(pkg, lit.Pos(), "Ports type %s must be a struct", typ)
	}

	protocols := protocolRoles(index)

	ports := make([]schema.Port, 0, st.NumFields())
	for i := range st.NumFields() {
		f := st.Field(i)

		group, ok := portFieldKind(f.Type())
		if !f.Exported() || !ok {
			return nil, posErrorf(pkg, f.Pos(),
				"Ports field %s must be an exported messaging.Port or "+
					"[]messaging.Port", f.Name())
		}

		roles, err := parsePortRoles(reflect.StructTag(st.Tag(i)).Get("akita"))
		if err != nil {
			return nil, posErrorf(pkg, f.Pos(), "Ports field %s: %v", f.Name(), err)
		}

		for _, r := range roles {
			if !protocols[r.Protocol][r.Role] {
				return nil, posErrorf(pkg, f.Pos(),
					"Ports field %s: no protocol %q with role %q is defined",
					f.Name(), r.Protocol, r.Role)
			}
		}

		ports = append(ports, schema.Port{Name: f.Name(), Roles: roles, Group: group})
	}

	return ports, nil
}

// portFieldKind reports whether typ is a port (messaging.Port) or a port
// group ([]messaging.Port).
func portFieldKind(typ types.Type) (group, ok bool) {
	if qualifiedTypeName(typ) == portTypeName {
		return false, true
	}

	if s, isSlice := typ.(*types.Slice); isSlice &&
		qualifiedTypeName(s.Elem()) == portTypeName {
		return true, true
	}

	return false, false
}

// middlewaresOfType lists the fields of a Middlewares struct in declaration
// order, which is the order the component runs them on every tick. Each field
// must be exported and implement modeling.Middleware.
func middlewaresOfType(
	pkg *packages.Package, lit *ast.CompositeLit, typ types.Type, index pkgIndex,
) ([]schema.Middleware, error) {
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, posErrorf(pkg, lit.Pos(), "Middlewares type %s must be a struct", typ)
	}

	iface := middlewareInterface(index)
	named, _ := types.Unalias(typ).(*types.Named)
	docs := fieldDocs(named, index)

	out := make([]schema.Middleware, 0, st.NumFields())
	for f := range st.Fields() {
		if !f.Exported() || (iface != nil && !types.Implements(f.Type(), iface)) {
			return nil, posErrorf(pkg, f.Pos(),
				"Middlewares field %s must be exported and implement "+
					"modeling.Middleware", f.Name())
		}

		out = append(out, schema.Middleware{
			Name: f.Name(),
			Type: types.TypeString(f.Type(), types.RelativeTo(pkg.Types)),
			Doc:  docs[f.Name()],
		})
	}

	return out, nil
}

// middlewareInterface returns the modeling.Middleware interface, or nil when
// the modeling package is not loaded.
func middlewareInterface(index pkgIndex) *types.Interface {
	modeling, ok := index[modelingPath]
	if !ok {
		return nil
	}

	obj := modeling.Types.Scope().Lookup(middlewareName)
	if obj == nil {
		return nil
	}

	iface, _ := obj.Type().Underlying().(*types.Interface)

	return iface
}
