package inspect

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

const defineProtocolFullName = "github.com/sarchlab/akita/v5/messaging.DefineProtocol"

// extractRoles resolves a []*messaging.Role{...} literal whose elements are
// references to package-level role vars declared in the fixed form
//
//	var <Name> = <Protocol>.Role("<role>")
//
// where <Protocol> is a package-level var initialized by
// messaging.DefineProtocol("<protocol>", ...). Anything else is an error:
// the convention is what makes roles statically resolvable.
func extractRoles(
	pkg *packages.Package, expr ast.Expr, index pkgIndex,
) ([]schema.Role, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, posErrorf(pkg, expr.Pos(),
			"Roles must be a slice literal of role identifiers")
	}

	roles := make([]schema.Role, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		role, err := resolveRole(pkg, elt, index)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, nil
}

// resolveRole follows a role identifier (e.g. memprotocol.Responder) to its
// declaration and recovers the protocol and role names.
func resolveRole(
	pkg *packages.Package, expr ast.Expr, index pkgIndex,
) (schema.Role, error) {
	obj, err := identObject(pkg, expr)
	if err != nil {
		return schema.Role{}, err
	}

	declPkg, ok := index[obj.Pkg().Path()]
	if !ok {
		return schema.Role{}, posErrorf(pkg, expr.Pos(),
			"cannot load package %s declaring role %s",
			obj.Pkg().Path(), obj.Name())
	}

	init := declOf(declPkg, obj.Name())
	if init == nil {
		return schema.Role{}, posErrorf(pkg, expr.Pos(),
			"cannot find declaration of role %s in %s",
			obj.Name(), obj.Pkg().Path())
	}

	// Expect <Protocol>.Role("<role>").
	call, ok := init.(*ast.CallExpr)
	if !ok {
		return schema.Role{}, roleConventionError(pkg, expr, obj)
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Role" || len(call.Args) != 1 {
		return schema.Role{}, roleConventionError(pkg, expr, obj)
	}

	roleName, err := constString(declPkg, call.Args[0])
	if err != nil {
		return schema.Role{}, err
	}

	protocolName, err := resolveProtocolName(declPkg, sel.X, index)
	if err != nil {
		return schema.Role{}, err
	}

	return schema.Role{Protocol: protocolName, Role: roleName}, nil
}

// resolveProtocolName follows a protocol var reference to its
// DefineProtocol call and returns the protocol's name.
func resolveProtocolName(
	pkg *packages.Package, expr ast.Expr, index pkgIndex,
) (string, error) {
	obj, err := identObject(pkg, expr)
	if err != nil {
		return "", err
	}

	declPkg, ok := index[obj.Pkg().Path()]
	if !ok {
		return "", posErrorf(pkg, expr.Pos(),
			"cannot load package %s declaring protocol %s",
			obj.Pkg().Path(), obj.Name())
	}

	init := declOf(declPkg, obj.Name())
	if init == nil {
		return "", posErrorf(pkg, expr.Pos(),
			"cannot find declaration of protocol %s in %s",
			obj.Name(), obj.Pkg().Path())
	}

	call, ok := init.(*ast.CallExpr)
	if !ok || len(call.Args) < 1 {
		return "", posErrorf(pkg, expr.Pos(),
			"protocol %s must be declared as "+
				"messaging.DefineProtocol(\"<name>\", ...)", obj.Name())
	}

	if fn := calleeFunc(declPkg, call); fn == nil ||
		fn.FullName() != defineProtocolFullName {
		return "", posErrorf(pkg, expr.Pos(),
			"protocol %s must be declared as "+
				"messaging.DefineProtocol(\"<name>\", ...)", obj.Name())
	}

	return constString(declPkg, call.Args[0])
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

// calleeFunc resolves a call's callee to the function object it invokes, or
// nil when it cannot.
func calleeFunc(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	fun := call.Fun

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

	fn, _ := pkg.TypesInfo.Uses[ident].(*types.Func)
	return fn
}

func roleConventionError(
	pkg *packages.Package, expr ast.Expr, obj types.Object,
) error {
	return posErrorf(pkg, expr.Pos(),
		"role %s.%s must be declared as a package-level "+
			"`var %s = <Protocol>.Role(\"<name>\")`",
		obj.Pkg().Path(), obj.Name(), obj.Name())
}
