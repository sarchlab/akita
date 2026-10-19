package inspect

import (
	"go/ast"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

const (
	messagingPath  = "github.com/sarchlab/akita/v5/sim/messaging"
	modelingPath   = "github.com/sarchlab/akita/v5/sim/modeling"
	portTypeName   = messagingPath + ".Port"
	middlewareName = "Middleware"
)

// extractModelDefinition reads the five element types from Comp and defaults
// from DefaultSpec. The resulting schema is independent of the declaration syntax.
func extractModelDefinition(
	pkg *packages.Package, component *types.Named, index pkgIndex, model string,
) (*schema.Definition, error) {
	args := make([]types.Type, 5)
	for i := range args {
		args[i] = component.TypeArgs().At(i)
	}
	specType, resType, portsType, mwType := args[0], args[2], args[3], args[4]
	if err := validateInitializers(pkg, component, args); err != nil {
		return nil, err
	}
	if err := validateSpecType(pkg, specType, index); err != nil {
		return nil, err
	}
	lit, err := defaultSpecLiteral(pkg, specType)
	if err != nil {
		return nil, err
	}
	defaults, err := evalStructLiteral(pkg, lit)
	if err != nil {
		return nil, err
	}
	def := &schema.Definition{Kind: schema.KindComponent, Model: model, Name: pkg.Types.Name()}
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
	if def.Middlewares, err = middlewaresOfType(pkg, lit, mwType, index, model); err != nil {
		return nil, err
	}
	if model == schema.ModelTicking {
		freq, _, _ := types.LookupFieldOrMethod(specType, false, pkg.Types, "Freq")
		if freq == nil || qualifiedTypeName(types.Unalias(freq.Type())) != "github.com/sarchlab/akita/v5/sim/timing.Freq" {
			return nil, posErrorf(pkg, lit.Pos(), "Spec %s must have a Freq timing.Freq field", specType)
		}
	}
	return def, nil
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
// `akita:"role=<protocol>.<role>"` tags and must name a defined protocol role.
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
// must be exported and implement that model's Middleware interface.
func middlewaresOfType(
	pkg *packages.Package, lit *ast.CompositeLit, typ types.Type, index pkgIndex, model string,
) ([]schema.Middleware, error) {
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, posErrorf(pkg, lit.Pos(), "Middlewares type %s must be a struct", typ)
	}

	iface := middlewareInterface(index, model)
	named, _ := types.Unalias(typ).(*types.Named)
	docs := fieldDocs(named, index)

	out := make([]schema.Middleware, 0, st.NumFields())
	for f := range st.Fields() {
		if !f.Exported() || (iface != nil && !types.Implements(f.Type(), iface)) {
			return nil, posErrorf(pkg, f.Pos(),
				"Middlewares field %s must be exported and implement "+
					"%s.Middleware", f.Name(), model)
		}

		out = append(out, schema.Middleware{
			Name: f.Name(),
			Type: types.TypeString(f.Type(), types.RelativeTo(pkg.Types)),
			Doc:  docs[f.Name()],
		})
	}

	return out, nil
}

// middlewareInterface returns the selected model's Middleware interface.
func middlewareInterface(index pkgIndex, model string) *types.Interface {
	modeling, ok := index[modelingPath+"/"+model]
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
