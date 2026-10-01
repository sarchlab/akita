package inspect

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

const defineProtocolFullName = "github.com/sarchlab/akita/v5/messaging.DefineProtocol"

// parsePortRoles parses a port's akita tag: comma-separated
// role=<protocol>.<role> directives, where <protocol> is the import path of
// the package that defines the protocol. Role names contain no dots, so the
// last dot separates the two.
func parsePortRoles(tag string) ([]schema.Role, error) {
	if tag == "" {
		return nil, nil
	}

	var roles []schema.Role
	for directive := range strings.SplitSeq(tag, ",") {
		key, value, _ := strings.Cut(directive, "=")
		dot := strings.LastIndex(value, ".")
		if key != "role" || dot <= 0 || dot == len(value)-1 ||
			strings.Contains(value[dot+1:], "/") {
			return nil, fmt.Errorf(
				"akita tag: want role=<protocol>.<role>, got %q", directive)
		}

		roles = append(roles, schema.Role{Protocol: value[:dot], Role: value[dot+1:]})
	}

	return roles, nil
}

// protocolRoles collects, from every loaded package, the protocols declared
// with messaging.DefineProtocol, keyed by the import path of the package that
// declares them (the protocol's name), and the names of their roles.
func protocolRoles(index pkgIndex) map[string]map[string]bool {
	out := map[string]map[string]bool{}

	for _, pkg := range index {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}

				fn := calleeFunc(pkg, call)
				if fn == nil || fn.FullName() != defineProtocolFullName {
					return true
				}

				roles := map[string]bool{}
				for _, arg := range call.Args {
					if role, ok := roleDefName(pkg, arg); ok {
						roles[role] = true
					}
				}
				out[pkg.PkgPath] = roles

				return true
			})
		}
	}

	return out
}

// roleDefName returns the Name of a messaging.RoleDef literal.
func roleDefName(pkg *packages.Package, expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return "", false
	}

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Name" {
			name, err := constString(pkg, kv.Value)
			return name, err == nil
		}
	}

	return "", false
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
